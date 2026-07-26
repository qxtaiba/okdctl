// Package nodetypes defines the shared domain vocabulary for cluster nodes
// and Proxmox VMs — NodeRole, VMState, NodeStatusPhase, the Kubernetes
// condition enums, the base CoreOS installer ISO name-shape allowlist, and
// the topology-to-IP ClusterNodes enumerator. It sits near the bottom of the
// import graph (config and low-level helpers only) so distribution phases,
// infrastructure providers, and the CLI can all import it without creating
// upward edges.
package nodetypes

import "fmt"

// NodeRole is the cluster-role assignment for an OKD node. Values are the
// lowercase strings openshift-install, HAProxy backend templates, and
// ignition URLs expect verbatim — change carefully.
type NodeRole string

// Node role values. String literals are load-bearing — openshift-install,
// HAProxy templates, and ignition URLs expect these verbatim.
const (
	RoleBootstrap NodeRole = "bootstrap"
	RoleMaster    NodeRole = "master"
	RoleWorker    NodeRole = "worker"
	RoleUnknown   NodeRole = "unknown"
)

// ParseNodeRole is the deserialization counterpart to NodeRole.String().
// Case-sensitive to match openshift-install output.
func ParseNodeRole(s string) (NodeRole, error) {
	switch NodeRole(s) {
	case RoleBootstrap, RoleMaster, RoleWorker:
		return NodeRole(s), nil
	default:
		return "", fmt.Errorf("unknown node role %q (want bootstrap|master|worker)", s)
	}
}

func (r NodeRole) String() string { return string(r) }
