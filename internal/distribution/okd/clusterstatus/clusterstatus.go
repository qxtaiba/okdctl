// Package clusterstatus aggregates live oc query output into
// okd.ClusterStatus: node readiness and role folding, degraded-operator
// counting, addon health, and cluster phase derivation.
package clusterstatus

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/qxtaiba/okdctl/internal/addon"
	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/distribution/okd"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
)

// Client is the slice of cluster.Client that status collection drives.
type Client interface {
	RawGet(ctx context.Context, path string) (string, error)
	GetJSON(ctx context.Context, args ...string) (stdout string, truncated bool, err error)
}

// AddonVerifier is the slice of addon.Manager used for addon health probes.
type AddonVerifier interface {
	VerifyAll(ctx context.Context) ([]addon.VerifyResult, error)
}

// statusNodeList is a minimal view of `oc get nodes -o json` for role +
// readiness parsing. Keeps the parse decoupled from corev1 schema evolution.
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
// for degraded-condition parsing. Reuses statusCondition for the conditions slice.
type statusClusterOperatorList struct {
	Items []statusClusterOperator `json:"items"`
}

type statusClusterOperator struct {
	Status struct {
		Conditions []statusCondition `json:"conditions"`
	} `json:"status"`
}

func (n *statusNode) isReady() bool {
	return slices.ContainsFunc(n.Status.Conditions, func(c statusCondition) bool {
		return c.Type == nodetypes.ConditionTypeReady && c.Status == nodetypes.ConditionStatusTrue
	})
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

// Collect queries the cluster for API reachability, node readiness, operator
// degradation, and addon health, then derives the overall phase. Failed oc
// queries degrade to empty sections rather than aborting so status renders
// whatever it can reach.
func Collect(ctx context.Context, cl Client, verifier AddonVerifier) okd.ClusterStatus {
	apiOK := true
	if _, ocErr := cl.RawGet(ctx, "/healthz"); ocErr != nil {
		apiOK = false
	}

	nodes := collectNodes(ctx, cl)
	degraded := countDegraded(ctx, cl)

	clusterPhase := okd.PhaseUnknown
	allReady := len(nodes) > 0 && !slices.ContainsFunc(nodes, func(n okd.NodeStatus) bool { return !n.Ready })
	switch {
	case apiOK && allReady && degraded == 0:
		clusterPhase = okd.PhaseRunning
	case apiOK && degraded > 0:
		clusterPhase = okd.PhaseDegraded
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
		Phase:             clusterPhase,
		APIReachable:      apiOK,
		Nodes:             nodes,
		DegradedOperators: degraded,
		Addons:            addonEntries,
	}
}

func collectNodes(ctx context.Context, cl Client) []okd.NodeStatus {
	nodesJSON, truncated, ocErr := cl.GetJSON(ctx, "get", "nodes", "-o", "json")
	if ocErr != nil {
		return nil
	}
	if truncated {
		tui.Warn("oc get nodes output truncated; node list may be incomplete")
	}
	var nl statusNodeList
	if jsonErr := json.Unmarshal([]byte(nodesJSON), &nl); jsonErr != nil {
		tui.Warn("oc get nodes json parse failed", tui.LF("err", jsonErr))
		return nil
	}
	var nodes []okd.NodeStatus
	for _, n := range nl.Items {
		ready := n.isReady()
		nodePhase := nodetypes.NodeStatusNotReady
		if ready {
			nodePhase = nodetypes.NodeStatusReady
		}
		nodes = append(nodes, okd.NodeStatus{
			Name:   n.Metadata.Name,
			Role:   n.role(),
			Ready:  ready,
			Status: nodePhase,
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
		tui.Warn("oc get clusteroperators output truncated; degraded count may be incomplete")
	}
	var col statusClusterOperatorList
	if jsonErr := json.Unmarshal([]byte(coJSON), &col); jsonErr != nil {
		tui.Warn("oc get clusteroperators json parse failed", tui.LF("err", jsonErr))
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
	workDir := filepath.Join(projectRoot, "okd-install")
	clusterDir := phase.ClusterConfigDir(workDir)
	kcPath := filepath.Join(clusterDir, "auth", "kubeconfig")

	if !system.FileExists(kcPath) {
		return nil, &errtypes.ClusterError{
			Msg: fmt.Sprintf("kubeconfig not found at %s; run `okdctl deploy` first", kcPath),
		}
	}

	return cluster.New(cluster.WithCLI("oc"), cluster.WithKubeconfig(kcPath)), nil
}
