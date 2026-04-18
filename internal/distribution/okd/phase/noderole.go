package phase

import "fmt"

// NodeRole is the cluster-role assignment for an OKD node. Values are the
// lowercase strings openshift-install, HAProxy backend templates, and
// ignition URLs expect verbatim — change carefully. Lives in phase/ rather
// than okd/ so subpackages (setup, install, destroy, cleanup) can use it
// without pulling an import cycle through okd → subpackage → okd.
type NodeRole string

const (
	RoleBootstrap NodeRole = "bootstrap"
	RoleMaster    NodeRole = "master"
	RoleWorker    NodeRole = "worker"
)

// ParseNodeRole converts a string to NodeRole, erroring on unknown values.
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
