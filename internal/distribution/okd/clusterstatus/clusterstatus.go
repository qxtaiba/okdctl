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
	"strings"
	"time"

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
	ListNodes(ctx context.Context) ([]cluster.NodeDetail, error)
	ClusterOperatorHealth(ctx context.Context) (cluster.OperatorHealth, error)
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

func projectNode(n *cluster.NodeDetail) okd.NodeStatus {
	status := nodetypes.NodeStatusUnknown
	switch nodetypes.ConditionStatus(n.ReadyCondition.Status) {
	case nodetypes.ConditionStatusTrue:
		status = nodetypes.NodeStatusReady
	case nodetypes.ConditionStatusFalse:
		status = nodetypes.NodeStatusNotReady
	}
	return okd.NodeStatus{Name: n.Name, Role: n.Role, Ready: n.Ready, Status: status}
}

// Collect queries the cluster for reachability, node readiness, operator
// degradation, and addon health, then derives the overall phase. cl may be
// nil before the first deploy — Collect then derives phase from lifecycle
// sources alone; failed oc queries otherwise degrade to empty sections
// rather than aborting.
func Collect(ctx context.Context, cl Client, verifier AddonVerifier, src LifecycleSources) okd.ClusterStatus {
	apiOK := false
	apiAvailable := cl != nil
	var apiLatency time.Duration
	var nodes []okd.NodeStatus
	nodesAvailable := false
	degraded := 0
	operatorsAvailable := false
	if cl != nil {
		started := time.Now()
		if _, ocErr := cl.RawGet(ctx, "/healthz"); ocErr == nil {
			apiOK = true
		}
		apiLatency = time.Since(started)
		nodes, nodesAvailable = collectNodes(ctx, cl)
		degraded, operatorsAvailable = countDegraded(ctx, cl)
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

	status := okd.ClusterStatus{
		Phase:               derivePhase(ctx, apiOK, nodes, degraded, src),
		APIReachable:        apiOK,
		APIAvailable:        apiAvailable,
		APILatencyAvailable: apiAvailable,
		APILatency:          apiLatency,
		Nodes:               nodes,
		DegradedOperators:   degraded,
		Addons:              addonEntries,
		NodesAvailable:      nodesAvailable,
		OperatorsAvailable:  operatorsAvailable,
	}
	if client, ok := cl.(*cluster.Client); ok {
		lastRun := readLastDeployRun(client.Kubeconfig, "")
		status.LastDeployRunID, status.LastDeployCluster, status.LastDeployAt = lastRun.RunID, lastRun.ClusterName, lastRun.At
	}
	return status
}

const (
	stepHistoryFileName = ".okdctl-step-history.json"
	stepHistoryVersion  = "v1"
)

type lastDeployRun struct {
	RunID       string
	ClusterName string
	At          time.Time
}

type deployHistoryHeader struct {
	SchemaVersion string    `json:"schema_version"`
	RunID         string    `json:"run_id"`
	Timestamp     time.Time `json:"timestamp"`
	ClusterName   string    `json:"cluster_name"`
}

func readLastDeployRun(kubeconfig, clusterName string) lastDeployRun {
	const suffix = "cluster-config/auth/kubeconfig"
	clean := filepath.Clean(kubeconfig)
	if !strings.HasSuffix(filepath.ToSlash(clean), "/"+suffix) {
		return lastDeployRun{}
	}
	workDir := filepath.Dir(filepath.Dir(filepath.Dir(clean)))
	if filepath.Base(workDir) != workspace.WorkDirName {
		return lastDeployRun{}
	}
	path := filepath.Join(workDir, stepHistoryFileName)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return lastDeployRun{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return lastDeployRun{}
	}
	var history deployHistoryHeader
	if json.Unmarshal(data, &history) != nil || history.SchemaVersion != stepHistoryVersion || history.RunID == "" || history.Timestamp.IsZero() || history.ClusterName == "" {
		return lastDeployRun{}
	}
	if clusterName != "" && history.ClusterName != clusterName {
		return lastDeployRun{}
	}
	return lastDeployRun{RunID: history.RunID, ClusterName: history.ClusterName, At: history.Timestamp}
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

func collectNodes(ctx context.Context, cl Client) ([]okd.NodeStatus, bool) {
	observations, err := cl.ListNodes(ctx)
	if err != nil {
		logutil.Warn("observe cluster nodes", logutil.LF("err", err))
		return nil, false
	}
	var nodes []okd.NodeStatus
	for i := range observations {
		nodes = append(nodes, projectNode(&observations[i]))
	}
	return nodes, true
}

func countDegraded(ctx context.Context, cl Client) (int, bool) {
	health, err := cl.ClusterOperatorHealth(ctx)
	if err != nil {
		logutil.Warn("observe cluster operators", logutil.LF("err", err))
		return 0, false
	}
	return len(health.Degraded), true
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
