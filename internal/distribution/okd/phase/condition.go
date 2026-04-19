package phase

// ConditionType and ConditionStatus mirror the Kubernetes status.conditions
// shape (Type / Status). Defined in phase/ rather than okd/ so subpackages
// (postinstall, install) can use the constants without pulling an import
// cycle through okd → subpackage → okd.
type ConditionType string

// Condition types mirroring the standard Kubernetes status.conditions values.
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
