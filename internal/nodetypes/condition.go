package nodetypes

// ConditionType and ConditionStatus mirror the Kubernetes status.conditions
// shape (Type / Status).
type ConditionType string

// Scaffolding: the three const groups below form the complete Kubernetes
// condition matrix (Ready/Available/Progressing/Degraded × True/False/Unknown)
// and are kept symmetric for the future status verb that surfaces non-Ready
// operator conditions. Removing the currently-unreferenced constants would
// lopside the enum and force a second PR when that caller lands.
const (
	ConditionTypeReady       ConditionType = "Ready"
	ConditionTypeAvailable   ConditionType = "Available"
	ConditionTypeProgressing ConditionType = "Progressing"
	ConditionTypeDegraded    ConditionType = "Degraded"
)

// ConditionStatus mirrors the Kubernetes status.conditions[*].status field.
type ConditionStatus string

// Condition status values mirroring standard Kubernetes conditions.
const (
	ConditionStatusTrue    ConditionStatus = "True"
	ConditionStatusFalse   ConditionStatus = "False"
	ConditionStatusUnknown ConditionStatus = "Unknown"
)

// NodeStatusPhase is the reported status of a node (the value a caller
// would print — distinct from the detailed Conditions slice).
type NodeStatusPhase string

// Node status-phase values surfaced when reporting node health.
const (
	NodeStatusReady    NodeStatusPhase = "Ready"
	NodeStatusNotReady NodeStatusPhase = "NotReady"
	NodeStatusUnknown  NodeStatusPhase = "Unknown"
)
