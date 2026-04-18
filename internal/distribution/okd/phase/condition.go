package phase

// ConditionType and ConditionStatus mirror the Kubernetes status.conditions
// shape (Type / Status). Defined in phase/ rather than okd/ so subpackages
// (postinstall, install) can use the constants without pulling an import
// cycle through okd → subpackage → okd.
type ConditionType string

const (
	ConditionTypeReady       ConditionType = "Ready"
	ConditionTypeAvailable   ConditionType = "Available"
	ConditionTypeProgressing ConditionType = "Progressing"
	ConditionTypeDegraded    ConditionType = "Degraded"
)

type ConditionStatus string

const (
	ConditionStatusTrue    ConditionStatus = "True"
	ConditionStatusFalse   ConditionStatus = "False"
	ConditionStatusUnknown ConditionStatus = "Unknown"
)

// NodeStatusPhase is the reported status of a node (the value a caller
// would print — distinct from the detailed Conditions slice).
type NodeStatusPhase string

const (
	NodeStatusReady    NodeStatusPhase = "Ready"
	NodeStatusNotReady NodeStatusPhase = "NotReady"
	NodeStatusUnknown  NodeStatusPhase = "Unknown"
)
