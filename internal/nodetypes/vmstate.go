package nodetypes

// VMState classifies a Proxmox VM's lifecycle state.
type VMState string

// Wire states: values match the "status" field in
// `pvesh get /nodes/<n>/qemu` output verbatim — change carefully.
// Proxmox emits only these two there; pause/suspend detail surfaces
// via the separate qmpstatus field, which okdctl does not read.
const (
	StateRunning VMState = "running"
	StateStopped VMState = "stopped"
)

// StateUnknown is a synthetic okdctl-side value, never emitted by
// Proxmox: it marks a VM whose status could not be determined.
const StateUnknown VMState = "unknown"
