package okd

import "github.com/qxtaiba/okdctl/internal/nodetypes"

// ClusterStatus is a read-only snapshot of an OKD cluster's state.
type ClusterStatus struct {
	Phase             ClusterPhase  `json:"phase"`
	APIReachable      bool          `json:"api_reachable"`
	Nodes             []NodeStatus  `json:"nodes,omitempty"`
	DegradedOperators int           `json:"degraded_operators"`
	Addons            []AddonStatus `json:"addons,omitempty"`
}

// AddonStatus is a health snapshot for a single registered addon.
type AddonStatus struct {
	Name    string `json:"name"`
	Healthy bool   `json:"healthy"`
	Error   string `json:"error,omitempty"`
}

// Label returns the user-visible health string for text output.
func (a AddonStatus) Label() string {
	switch {
	case a.Healthy:
		return "healthy"
	case a.Error != "":
		return "degraded"
	default:
		return "not enabled"
	}
}

// ClusterPhase is the high-level lifecycle state the CLI renders in status
// output. Values are title-cased to match the conventions used by kube-style
// tooling.
type ClusterPhase string

const (
	// PhasePending is the pre-install state (config loaded, no infra yet).
	PhasePending ClusterPhase = "Pending"
	// PhaseInstalling covers terraform apply + bootstrap + install monitor.
	PhaseInstalling ClusterPhase = "Installing"
	// PhaseRunning means the cluster is up and all operators report healthy.
	PhaseRunning ClusterPhase = "Running"
	// PhaseDegraded means the cluster serves API traffic but one or more
	// operators report a non-healthy condition or a node is not ready.
	PhaseDegraded ClusterPhase = "Degraded"
	// PhaseStopped means provisioned VMs exist but every one reports powered
	// off — the state 'okdctl cluster stop' leaves behind.
	PhaseStopped ClusterPhase = "Stopped"
	// PhaseUnknown is the default when status cannot be determined.
	PhaseUnknown ClusterPhase = "Unknown"
)

// NodeStatus is one cluster node's projected identity and health.
type NodeStatus struct {
	Name   string                    `json:"name"`
	Role   nodetypes.NodeRole        `json:"role"`
	Ready  bool                      `json:"ready"`
	Status nodetypes.NodeStatusPhase `json:"status,omitempty"`
}
