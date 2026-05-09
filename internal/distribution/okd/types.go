package okd

import "github.com/qxtaiba/okdctl/internal/distribution/okd/phase"

// ClusterStatus is a read-only snapshot of an OKD cluster — phase, version,
// endpoints, per-node conditions, and a free-form Message used by CLI status
// commands.
type ClusterStatus struct {
	Phase             ClusterPhase  `json:"phase"`
	APIReachable      bool          `json:"api_reachable"`
	Version           string        `json:"version,omitempty"`
	APIServerURL      string        `json:"api_server_url,omitempty"`
	ConsoleURL        string        `json:"console_url,omitempty"`
	Nodes             []NodeStatus  `json:"nodes"`
	DegradedOperators int           `json:"degraded_operators"`
	Conditions        []Condition   `json:"conditions,omitempty"`
	Addons            []AddonStatus `json:"addons,omitempty"`
	Message           string        `json:"message,omitempty"`
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
// output and uses for exit-code mapping. Values are title-cased to match the
// conventions used by kube-style tooling.
type ClusterPhase string

const (
	// PhasePending is the pre-install state (config loaded, no infra yet).
	PhasePending ClusterPhase = "Pending"
	// PhaseInstalling covers terraform apply + bootstrap + install monitor.
	PhaseInstalling ClusterPhase = "Installing"
	// PhaseRunning means the cluster is up and all operators report healthy.
	PhaseRunning ClusterPhase = "Running"
	// PhaseDegraded means the cluster serves API traffic but one or more
	// operators report a non-healthy condition.
	PhaseDegraded ClusterPhase = "Degraded"
	// PhaseFailed is a terminal install failure requiring destroy+retry.
	PhaseFailed ClusterPhase = "Failed"
	// PhaseUnknown is the default when status cannot be determined.
	PhaseUnknown ClusterPhase = "Unknown"
)

// NodeStatus is one cluster node's projected identity and health.
type NodeStatus struct {
	Name       string                `json:"name"`
	Role       phase.NodeRole        `json:"role"`
	Ready      bool                  `json:"ready"`
	Status     phase.NodeStatusPhase `json:"status,omitempty"`
	Version    string                `json:"version,omitempty"`
	InternalIP string                `json:"internal_ip,omitempty"`
	Conditions []Condition           `json:"conditions,omitempty"`
}

// Condition mirrors the k8s condition shape but carries project-local
// ConditionType/Status values from internal/distribution/okd/phase.
type Condition struct {
	Type    phase.ConditionType   `json:"type"`
	Status  phase.ConditionStatus `json:"status"`
	Reason  string                `json:"reason,omitempty"`
	Message string                `json:"message,omitempty"`
}
