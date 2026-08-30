// Package clusterstatus aggregates live oc query output into
// okd.ClusterStatus: node readiness and role folding, degraded-operator
// counting, addon health, and cluster phase derivation.
package clusterstatus

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/qxtaiba/okdctl/internal/addon"
	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/distribution/okd"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

// Client is the slice of cluster.Client that status collection drives.
type Client interface {
	RawGet(ctx context.Context, path string) (string, error)
	GetJSON(ctx context.Context, args ...string) (stdout string, truncated bool, err error)
}

// PowerProber reports Proxmox VM power state keyed by vmid.
type PowerProber interface {
	VMStates(ctx context.Context) (map[int]nodetypes.VMState, error)
}

// LifecycleSources carries non-API lifecycle signals consulted when the API
// is unreachable; a nil field degrades derivation to a less specific phase.
type LifecycleSources struct {
	// DeployInProgress reports an unfinished deploy for this cluster
	// (deploy.InstallInProgress).
	DeployInProgress func() bool
	// InfraPresent reports whether the configured terraform environment
	// has provisioned resources (TerraformStateHasResources).
	InfraPresent func() bool
	// Power probes VM power states; nil when no Proxmox credentials resolve.
	Power PowerProber
}

// AddonVerifier is the slice of addon.Manager used for addon health probes.
type AddonVerifier interface {
	VerifyAll(ctx context.Context) ([]addon.VerifyResult, error)
}

// statusNodeList is a minimal view of `oc get nodes -o json`, decoupled
// from corev1 schema evolution.
type statusNodeList struct {
	Items []statusNode `json:"items"`
}

type statusCondition struct {
	Type   nodetypes.ConditionType   `json:"type"`
	Status nodetypes.ConditionStatus `json:"status"`
}

type statusNode struct {
	Metadata struct {
		Name   string            `json:"name"`
		Labels map[string]string `json:"labels"`
	} `json:"metadata"`
	Status struct {
		Conditions []statusCondition `json:"conditions"`
	} `json:"status"`
}

// statusClusterOperatorList is a minimal view of `oc get clusteroperators -o json`
// for degraded-condition parsing.
type statusClusterOperatorList struct {
	Items []statusClusterOperator `json:"items"`
}

type statusClusterOperator struct {
	Status struct {
		Conditions []statusCondition `json:"conditions"`
	} `json:"status"`
}

func (n *statusNode) readyCondition() nodetypes.ConditionStatus {
	for _, c := range n.Status.Conditions {
		if c.Type == nodetypes.ConditionTypeReady {
			return c.Status
		}
	}
	return nodetypes.ConditionStatusUnknown
}

func (n *statusNode) isReady() bool {
	return n.readyCondition() == nodetypes.ConditionStatusTrue
}

func (n *statusNode) statusPhase() nodetypes.NodeStatusPhase {
	switch n.readyCondition() {
	case nodetypes.ConditionStatusTrue:
		return nodetypes.NodeStatusReady
	case nodetypes.ConditionStatusFalse:
		return nodetypes.NodeStatusNotReady
	default:
		return nodetypes.NodeStatusUnknown
	}
}

func (n *statusNode) role() nodetypes.NodeRole {
	if _, ok := n.Metadata.Labels["node-role.kubernetes.io/master"]; ok {
		return nodetypes.RoleMaster
	}
	if _, ok := n.Metadata.Labels["node-role.kubernetes.io/worker"]; ok {
		return nodetypes.RoleWorker
	}
	return nodetypes.RoleUnknown
}

// ParseNode parses a single `oc get node <name> -o json` document into the
// node's projected identity and readiness.
func ParseNode(data []byte) (okd.NodeStatus, error) {
	var n statusNode
	if err := json.Unmarshal(data, &n); err != nil {
		return okd.NodeStatus{}, fmt.Errorf("parse node json: %w", err)
	}
	return okd.NodeStatus{Name: n.Metadata.Name, Role: n.role(), Ready: n.isReady()}, nil
}

// Collect queries the cluster for reachability, node readiness, operator
// degradation, and addon health, then derives the overall phase. cl may be
// nil before the first deploy — Collect then derives phase from lifecycle
// sources alone; failed oc queries otherwise degrade to empty sections
// rather than aborting.
func Collect(ctx context.Context, cl Client, verifier AddonVerifier, src LifecycleSources) okd.ClusterStatus {
	apiOK := false
	var nodes []okd.NodeStatus
	degraded := 0
	if cl != nil {
		if _, ocErr := cl.RawGet(ctx, "/healthz"); ocErr == nil {
			apiOK = true
		}
		nodes = collectNodes(ctx, cl)
		degraded = countDegraded(ctx, cl)
	}

	addonResults, _ := verifier.VerifyAll(ctx)
	var addonEntries []okd.AddonStatus
	for _, r := range addonResults {
		e := okd.AddonStatus{Name: r.Name, Healthy: r.Err == nil}
		if r.Err != nil {
			e.Error = r.Err.Error()
		}
		addonEntries = append(addonEntries, e)
	}

	return okd.ClusterStatus{
		Phase:             derivePhase(ctx, apiOK, nodes, degraded, src),
		APIReachable:      apiOK,
		Nodes:             nodes,
		DegradedOperators: degraded,
		Addons:            addonEntries,
	}
}

// derivePhase maps lifecycle signals to ClusterPhase, checking cheapest
// signals first: live API, then local markers, then the Proxmox power probe.
func derivePhase(ctx context.Context, apiOK bool, nodes []okd.NodeStatus, degraded int, src LifecycleSources) okd.ClusterPhase {
	if apiOK {
		switch {
		case degraded > 0:
			return okd.PhaseDegraded
		case len(nodes) == 0:
			return okd.PhaseUnknown
		case slices.ContainsFunc(nodes, func(n okd.NodeStatus) bool { return !n.Ready }):
			return okd.PhaseDegraded
		default:
			return okd.PhaseRunning
		}
	}
	if src.DeployInProgress != nil && src.DeployInProgress() {
		return okd.PhaseInstalling
	}
	if src.InfraPresent == nil || !src.InfraPresent() {
		return okd.PhasePending
	}
	return powerPhase(ctx, src.Power)
}

// powerPhase classifies infra-present-but-API-down via the VM power probe.
func powerPhase(ctx context.Context, prober PowerProber) okd.ClusterPhase {
	if prober == nil {
		return okd.PhaseUnknown
	}
	states, err := prober.VMStates(ctx)
	if err != nil {
		logutil.Warn("proxmox power probe failed; phase stays unknown", logutil.LF("err", err))
		return okd.PhaseUnknown
	}
	if len(states) == 0 {
		return okd.PhaseUnknown
	}
	for _, s := range states {
		if s != nodetypes.StateStopped {
			return okd.PhaseUnknown
		}
	}
	return okd.PhaseStopped
}

// TerraformStateHasResources reports whether tfEnv's terraform state under
// projectRoot records at least one resource; an empty post-destroy state or
// another environment's state does not count.
func TerraformStateHasResources(projectRoot, tfEnv string) bool {
	data, err := os.ReadFile(
		filepath.Join(workspace.TerraformEnvDir(projectRoot, tfEnv), "terraform.tfstate"))
	if err != nil {
		return false
	}
	var st struct {
		Resources []json.RawMessage `json:"resources"`
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return false
	}
	return len(st.Resources) > 0
}

func collectNodes(ctx context.Context, cl Client) []okd.NodeStatus {
	nodesJSON, truncated, ocErr := cl.GetJSON(ctx, "get", "nodes", "-o", "json")
	if ocErr != nil {
		return nil
	}
	if truncated {
		logutil.Warn("oc get nodes output truncated; node list may be incomplete")
	}
	var nl statusNodeList
	if jsonErr := json.Unmarshal([]byte(nodesJSON), &nl); jsonErr != nil {
		logutil.Warn("oc get nodes json parse failed", logutil.LF("err", jsonErr))
		return nil
	}
	var nodes []okd.NodeStatus
	for _, n := range nl.Items {
		nodes = append(nodes, okd.NodeStatus{
			Name:   n.Metadata.Name,
			Role:   n.role(),
			Ready:  n.isReady(),
			Status: n.statusPhase(),
		})
	}
	return nodes
}

func countDegraded(ctx context.Context, cl Client) int {
	coJSON, truncated, ocErr := cl.GetJSON(ctx, "get", "clusteroperators", "-o", "json")
	if ocErr != nil {
		return 0
	}
	if truncated {
		logutil.Warn("oc get clusteroperators output truncated; degraded count may be incomplete")
	}
	var col statusClusterOperatorList
	if jsonErr := json.Unmarshal([]byte(coJSON), &col); jsonErr != nil {
		logutil.Warn("oc get clusteroperators json parse failed", logutil.LF("err", jsonErr))
		return 0
	}
	degraded := 0
	for _, co := range col.Items {
		if slices.ContainsFunc(co.Status.Conditions, func(c statusCondition) bool {
			return c.Type == nodetypes.ConditionTypeDegraded && c.Status == nodetypes.ConditionStatusTrue
		}) {
			degraded++
		}
	}
	return degraded
}

// NewClient returns an oc-backed cluster client for the deployed cluster, or
// a ClusterError when no kubeconfig exists under <projectRoot>/okd-install yet.
func NewClient(projectRoot string) (*cluster.Client, error) {
	workDir := workspace.WorkDir(projectRoot)
	clusterDir := workspace.ClusterConfigDir(workDir)
	kcPath := workspace.KubeconfigPath(clusterDir)

	if !system.FileExists(kcPath) {
		return nil, &errtypes.ClusterError{
			Msg: fmt.Sprintf("kubeconfig not found at %s; run `okdctl deploy` first", kcPath),
		}
	}

	return cluster.New(cluster.WithCLI("oc"), cluster.WithKubeconfig(kcPath)), nil
}
