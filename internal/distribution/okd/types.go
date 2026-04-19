package okd

import "github.com/qxtaiba/okdctl/internal/distribution/okd/phase"

// ClusterStatus is a read-only snapshot of an OKD cluster — phase, version,
// endpoints, per-node conditions, and a free-form Message used by CLI status
// commands.
type ClusterStatus struct {
	Phase        ClusterPhase
	Version      string
	APIServerURL string
	ConsoleURL   string
	Nodes        []NodeStatus
	Conditions   []Condition
	Message      string
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
	Name       string
	Role       phase.NodeRole
	Status     phase.NodeStatusPhase
	Version    string
	InternalIP string
	Conditions []Condition
}

// Condition mirrors the k8s condition shape but carries project-local
// ConditionType/Status values from internal/distribution/okd/phase.
type Condition struct {
	Type    phase.ConditionType
	Status  phase.ConditionStatus
	Reason  string
	Message string
}
