package okd

import "github.com/qxtaiba/okdctl/internal/nodetypes"

// ClusterStatus is a read-only snapshot of an OKD cluster: lifecycle phase,
// API reachability, per-node health, operator degradation count, and addon
// health results.
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
// Healthy → "healthy"; unhealthy with an error → "degraded";
// zero-value (not in verify results) → "not enabled".
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
	Name       string                    `json:"name"`
	Role       nodetypes.NodeRole        `json:"role"`
	Ready      bool                      `json:"ready"`
	Status     nodetypes.NodeStatusPhase `json:"status,omitempty"`
	Version    string                    `json:"version,omitempty"`
	InternalIP string                    `json:"internal_ip,omitempty"`
	Conditions []Condition               `json:"conditions,omitempty"`
}

// Condition mirrors the k8s condition shape but carries project-local
// ConditionType/Status values from internal/distribution/okd/phase.
type Condition struct {
	Type    nodetypes.ConditionType   `json:"type"`
	Status  nodetypes.ConditionStatus `json:"status"`
	Reason  string                    `json:"reason,omitempty"`
	Message string                    `json:"message,omitempty"`
}
