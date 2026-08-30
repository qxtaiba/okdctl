package nodetypes

// VMState classifies a Proxmox VM's lifecycle state.
type VMState string

// Wire states matching pvesh's "status" field verbatim — change carefully.
// Proxmox reports only these two; pause/suspend surface via the separate
// qmpstatus field (not read here).
const (
	StateRunning VMState = "running"
	StateStopped VMState = "stopped"
)

// StateUnknown is a synthetic okdctl-side value (never emitted by Proxmox)
// marking a VM whose status could not be determined.
const StateUnknown VMState = "unknown"
