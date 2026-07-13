package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

// DrainOptions tunes a node drain. Force allows eviction of pods not managed
// by a controller (bare pods); without it drain refuses such pods. Timeout is
// passed verbatim to `oc adm drain --timeout` (e.g. "10m"); empty means no
// client-side timeout.
type DrainOptions struct {
	Force            bool
	Timeout          string
	DeleteEmptyDir   bool
	IgnoreDaemonsets bool
}

// Cordon marks node unschedulable via `oc adm cordon`. Idempotent: cordoning
// an already-cordoned node exits 0.
func (c *Client) Cordon(ctx context.Context, node string) error {
	if err := c.runCheck(ctx, "adm", "cordon", node); err != nil {
		return &errtypes.ClusterError{Msg: fmt.Sprintf("cordon node %s", node), Err: err}
	}
	return nil
}

// Uncordon marks node schedulable again via `oc adm uncordon`. Idempotent.
func (c *Client) Uncordon(ctx context.Context, node string) error {
	if err := c.runCheck(ctx, "adm", "uncordon", node); err != nil {
		return &errtypes.ClusterError{Msg: fmt.Sprintf("uncordon node %s", node), Err: err}
	}
	return nil
}

// Drain evicts pods off node via `oc adm drain`. It always cordons first
// (drain's own behavior), so calling Cordon beforehand only makes the intent
// explicit. Re-running against an already-drained node is a no-op.
func (c *Client) Drain(ctx context.Context, node string, opts DrainOptions) error {
	args := []string{"adm", "drain", node}
	if opts.IgnoreDaemonsets {
		args = append(args, "--ignore-daemonsets")
	}
	if opts.DeleteEmptyDir {
		args = append(args, "--delete-emptydir-data")
	}
	if opts.Force {
		args = append(args, "--force")
	}
	if opts.Timeout != "" {
		args = append(args, "--timeout="+opts.Timeout)
	}
	if err := c.runCheck(ctx, args...); err != nil {
		return &errtypes.ClusterError{Msg: fmt.Sprintf("drain node %s", node), Err: err}
	}
	return nil
}

// DeleteNode removes the Node object via `oc delete node --ignore-not-found`,
// so a re-run after the object is already gone succeeds. This deletes only the
// Kubernetes registration; the VM teardown is a separate terraform step.
func (c *Client) DeleteNode(ctx context.Context, node string) error {
	if err := c.runCheck(ctx, "delete", "node", node, "--ignore-not-found"); err != nil {
		return &errtypes.ClusterError{Msg: fmt.Sprintf("delete node %s", node), Err: err}
	}
	return nil
}

// SetMastersSchedulable patches the cluster Scheduler so control-plane nodes
// accept regular workloads (or stop accepting them). Required before draining
// the last workers in a compaction so pods and ingress have somewhere to land.
func (c *Client) SetMastersSchedulable(ctx context.Context, schedulable bool) error {
	patch := fmt.Sprintf(`{"spec":{"mastersSchedulable":%t}}`, schedulable)
	if err := c.runCheck(ctx, "patch", "schedulers.config.openshift.io", "cluster",
		"--type=merge", "-p", patch); err != nil {
		return &errtypes.ClusterError{Msg: "patch scheduler mastersSchedulable", Err: err}
	}
	return nil
}

// MastersSchedulable reports the cluster Scheduler's spec.mastersSchedulable.
func (c *Client) MastersSchedulable(ctx context.Context) (bool, error) {
	data, err := c.getJSONChecked(ctx, "get scheduler", "get", "schedulers.config.openshift.io", "cluster", "-o", "json")
	if err != nil {
		return false, err
	}
	return parseMastersSchedulable(data)
}

func parseMastersSchedulable(data []byte) (bool, error) {
	var s struct {
		Spec struct {
			MastersSchedulable bool `json:"mastersSchedulable"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return false, fmt.Errorf("parse scheduler json: %w", err)
	}
	return s.Spec.MastersSchedulable, nil
}

// NodeDetail is the projected identity of a cluster node used by lifecycle
// guards: name, role, readiness, and the trailing-integer index parsed from
// the name (worker2 → 2), which maps a node to its terraform count index.
type NodeDetail struct {
	Name  string
	Role  nodetypes.NodeRole
	Ready bool
}

// ListNodes returns every node's projected identity from `oc get nodes -o json`.
func (c *Client) ListNodes(ctx context.Context) ([]NodeDetail, error) {
	data, err := c.getJSONChecked(ctx, "list nodes", "get", "nodes", "-o", "json")
	if err != nil {
		return nil, err
	}
	return parseNodeList(data)
}

func parseNodeList(data []byte) ([]NodeDetail, error) {
	var nl struct {
		Items []struct {
			Metadata struct {
				Name   string            `json:"name"`
				Labels map[string]string `json:"labels"`
			} `json:"metadata"`
			Status struct {
				Conditions []struct {
					Type   nodetypes.ConditionType   `json:"type"`
					Status nodetypes.ConditionStatus `json:"status"`
				} `json:"conditions"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &nl); err != nil {
		return nil, fmt.Errorf("parse node list json: %w", err)
	}
	out := make([]NodeDetail, 0, len(nl.Items))
	for _, item := range nl.Items {
		d := NodeDetail{Name: item.Metadata.Name, Role: nodetypes.RoleUnknown}
		if _, ok := item.Metadata.Labels["node-role.kubernetes.io/master"]; ok {
			d.Role = nodetypes.RoleMaster
		} else if _, ok := item.Metadata.Labels["node-role.kubernetes.io/worker"]; ok {
			d.Role = nodetypes.RoleWorker
		}
		for _, cond := range item.Status.Conditions {
			if cond.Type == nodetypes.ConditionTypeReady && cond.Status == nodetypes.ConditionStatusTrue {
				d.Ready = true
			}
		}
		out = append(out, d)
	}
	return out, nil
}

// PodPlacement is a pod's identity and the node it is scheduled on, used by
// storage/ingress guards to decide whether removing a node destroys data or
// strands ingress.
type PodPlacement struct {
	Name      string
	Namespace string
	NodeName  string
}

// PodsForSelector lists pods matching selector and their node placement.
// namespace "" queries all namespaces (`-A`); selector is a label selector
// (e.g. "app=rook-ceph-osd"), empty for none.
func (c *Client) PodsForSelector(ctx context.Context, namespace, selector string) ([]PodPlacement, error) {
	args := []string{"get", "pods"}
	if namespace == "" {
		args = append(args, "-A")
	} else {
		args = append(args, "-n", namespace)
	}
	if selector != "" {
		args = append(args, "-l", selector)
	}
	args = append(args, "-o", "json")

	data, err := c.getJSONChecked(ctx, "list pods", args...)
	if err != nil {
		return nil, err
	}
	return parsePodPlacements(data)
}

func parsePodPlacements(data []byte) ([]PodPlacement, error) {
	var pl struct {
		Items []struct {
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
			Spec struct {
				NodeName string `json:"nodeName"`
			} `json:"spec"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &pl); err != nil {
		return nil, fmt.Errorf("parse pod list json: %w", err)
	}
	out := make([]PodPlacement, 0, len(pl.Items))
	for _, p := range pl.Items {
		out = append(out, PodPlacement{
			Name:      p.Metadata.Name,
			Namespace: p.Metadata.Namespace,
			NodeName:  p.Spec.NodeName,
		})
	}
	return out, nil
}

// Apply applies a manifest via `oc apply -f -`, feeding it on stdin. cluster
// had no stdin primitive before this; node-lifecycle's compact-ingress step is
// the first non-phase caller that needs one.
func (c *Client) Apply(ctx context.Context, manifest []byte) error {
	result, err := c.exec.RunWithStdin(ctx, string(manifest), c.CLI, "apply", "-f", "-")
	if err != nil {
		return &errtypes.ClusterError{Msg: "apply manifest", Err: err}
	}
	if result.ExitCode != 0 {
		return &errtypes.ClusterError{Msg: "apply manifest", Err: executor.NewExitError(ctx, c.CLI+" apply -f -", result.ExitCode, result.Stderr)}
	}
	return nil
}

// NodeIndex extracts the trailing integer of a node name (worker2 → 2, true).
// okdctl VMs are numbered per role, so the suffix is the terraform count index;
// a name with no trailing digits returns (0, false). Kubernetes reports nodes
// by FQDN (grappleberry-worker0.grappleberry.k8s.local), so the domain is
// stripped first — the index lives in the trailing digits of the first label.
func NodeIndex(name string) (int, bool) {
	if dot := strings.IndexByte(name, '.'); dot != -1 {
		name = name[:dot]
	}
	i := len(name)
	for i > 0 && name[i-1] >= '0' && name[i-1] <= '9' {
		i--
	}
	if i == len(name) {
		return 0, false
	}
	n, err := strconv.Atoi(name[i:])
	if err != nil {
		return 0, false
	}
	return n, true
}
