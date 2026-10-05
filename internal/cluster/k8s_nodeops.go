package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

// DrainOptions tunes a node drain: Force evicts pods not owned by a
// controller; empty Timeout means no client-side timeout.
type DrainOptions struct {
	Force            bool
	Timeout          string
	DeleteEmptyDir   bool
	IgnoreDaemonsets bool
}

// Cordon marks node unschedulable via `oc adm cordon`; idempotent.
func (c *Client) Cordon(ctx context.Context, node string) error {
	if err := c.runCheck(ctx, "adm", "cordon", node); err != nil {
		return &errtypes.ClusterError{Msg: fmt.Sprintf("cordon node %s", node), Err: err}
	}
	return nil
}

// Uncordon marks node schedulable again via `oc adm uncordon`; idempotent.
func (c *Client) Uncordon(ctx context.Context, node string) error {
	if err := c.runCheck(ctx, "adm", "uncordon", node); err != nil {
		return &errtypes.ClusterError{Msg: fmt.Sprintf("uncordon node %s", node), Err: err}
	}
	return nil
}

// Drain evicts pods off node via `oc adm drain`, which cordons first;
// re-running against an already-drained node is a no-op.
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

// DeleteNode removes the Node object via `oc delete node --ignore-not-found`
// (idempotent); it deletes only the k8s registration, not the VM.
func (c *Client) DeleteNode(ctx context.Context, node string) error {
	if err := c.runCheck(ctx, "delete", "node", node, "--ignore-not-found"); err != nil {
		return &errtypes.ClusterError{Msg: fmt.Sprintf("delete node %s", node), Err: err}
	}
	return nil
}

// SetMastersSchedulable patches the cluster Scheduler so control-plane nodes
// accept (or stop accepting) regular workloads — required before draining the
// last workers in a compaction.
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

// NodeDetail is a cluster node's projected identity — name, role, readiness —
// used by lifecycle guards.
type NodeDetail struct {
	Name           string
	Role           nodetypes.NodeRole
	Ready          bool
	ReadyCondition corev1.NodeCondition
}

// ListNodes returns every node's projected identity from `oc get nodes -o json`.
func (c *Client) ListNodes(ctx context.Context) ([]NodeDetail, error) {
	data, err := c.getJSONChecked(ctx, "list nodes", "get", "nodes", "-o", "json")
	if err != nil {
		return nil, err
	}
	return parseNodeList(data)
}

// ParseNode preserves the full Ready condition, defaulting absent readiness to Unknown.
func ParseNode(data []byte) (NodeDetail, error) {
	var n corev1.Node
	if err := json.Unmarshal(data, &n); err != nil {
		return NodeDetail{}, fmt.Errorf("parse node json: %w", err)
	}
	return observeNode(&n), nil
}

func parseNodeList(data []byte) ([]NodeDetail, error) {
	var list corev1.NodeList
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("parse node list json: %w", err)
	}
	out := make([]NodeDetail, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, observeNode(&list.Items[i]))
	}
	return out, nil
}

func observeNode(n *corev1.Node) NodeDetail {
	d := NodeDetail{Name: n.Name, Role: nodetypes.RoleUnknown, ReadyCondition: corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionUnknown}}
	_, master := n.Labels["node-role.kubernetes.io/master"]
	_, controlPlane := n.Labels["node-role.kubernetes.io/control-plane"]
	if master || controlPlane {
		d.Role = nodetypes.RoleMaster
	} else if _, worker := n.Labels["node-role.kubernetes.io/worker"]; worker {
		d.Role = nodetypes.RoleWorker
	}
	for _, condition := range n.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			d.ReadyCondition = condition
			break
		}
	}
	d.Ready = d.ReadyCondition.Status == corev1.ConditionTrue
	return d
}

// PodPlacement is a pod's identity and scheduled node, used by storage/
// ingress guards to detect whether removing a node destroys data or strands
// ingress.
type PodPlacement struct {
	Name      string
	Namespace string
	NodeName  string
}

// PodsForSelector lists pods matching selector and their node placement;
// namespace "" queries all namespaces, selector "" matches all pods.
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

// Apply applies a manifest via `oc apply -f -`, feeding it on stdin.
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

// NodeIndex extracts the trailing integer from a node name (worker2 → 2, true).
// The domain is stripped first since kubernetes reports nodes by FQDN.
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
