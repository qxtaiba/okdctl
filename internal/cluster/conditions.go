package cluster

// ConditionType mirrors the Kubernetes status.conditions[*].type field.
type ConditionType string

// Condition types read from ClusterOperator and Node status.
const (
	ConditionTypeReady       ConditionType = "Ready"
	ConditionTypeAvailable   ConditionType = "Available"
	ConditionTypeProgressing ConditionType = "Progressing"
	ConditionTypeDegraded    ConditionType = "Degraded"
)

// ConditionStatus mirrors the Kubernetes status.conditions[*].status field.
type ConditionStatus string

// ConditionStatusTrue is the only status the health gates test for.
const ConditionStatusTrue ConditionStatus = "True"
