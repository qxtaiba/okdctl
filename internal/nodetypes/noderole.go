// Package nodetypes defines the shared domain vocabulary for cluster nodes
// and Proxmox VMs — NodeRole, VMState, NodeStatusPhase, and the Kubernetes
// condition enums. It is a leaf package (stdlib-only) so distribution
// phases, infrastructure providers, and the CLI can all import it without
// creating upward edges in the import graph.
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
// Currently no caller; retained as the canonical symmetric pair for
// upcoming status JSON / terraform-output deserialization. Case-sensitive
// to match openshift-install output.
func ParseNodeRole(s string) (NodeRole, error) {
	switch NodeRole(s) {
	case RoleBootstrap, RoleMaster, RoleWorker:
		return NodeRole(s), nil
	default:
		return "", fmt.Errorf("unknown node role %q (want bootstrap|master|worker)", s)
	}
}

func (r NodeRole) String() string { return string(r) }
