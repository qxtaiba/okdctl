package nodetypes

// VMState classifies a Proxmox VM's lifecycle state. Values match the
// "status" field in `pvesh get /nodes/<n>/qemu` output verbatim — change
// carefully.
type VMState string

// Scaffolding (api:3e02f6b8): the full state matrix is kept as a single
// source of truth for the future status path that surfaces partial-running
// clusters. Today only StateRunning is consumed (proxmox.Provider mapping).
const (
	StateRunning  VMState = "running"
	StateStopped  VMState = "stopped"
	StateCreating VMState = "creating"
	StateDeleting VMState = "deleting"
	StateUnknown  VMState = "unknown"
)
