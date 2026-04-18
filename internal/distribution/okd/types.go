package okd

import "github.com/qxtaiba/okdctl/internal/distribution/okd/phase"

type ClusterStatus struct {
	Phase        ClusterPhase
	Version      string
	APIServerURL string
	ConsoleURL   string
	Nodes        []NodeStatus
	Conditions   []Condition
	Message      string
}

type ClusterPhase string

const (
	PhasePending    ClusterPhase = "Pending"
	PhaseInstalling ClusterPhase = "Installing"
	PhaseRunning    ClusterPhase = "Running"
	PhaseDegraded   ClusterPhase = "Degraded"
	PhaseFailed     ClusterPhase = "Failed"
	PhaseUnknown    ClusterPhase = "Unknown"
)

type NodeStatus struct {
	Name       string
	Role       phase.NodeRole
	Status     phase.NodeStatusPhase
	Version    string
	InternalIP string
	Conditions []Condition
}

type Condition struct {
	Type    phase.ConditionType
	Status  phase.ConditionStatus
	Reason  string
	Message string
}
