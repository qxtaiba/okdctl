package phase

// VMState classifies a Proxmox VM's lifecycle state. Values match the
// "status" field in `pvesh get /nodes/<n>/qemu` output verbatim — change
// carefully. Lives in phase/ rather than infrastructure/proxmox so
// iso_cleanup (phase) and proxmox can share a single source of truth
// without an import cycle (proxmox already imports phase for NodeRole).
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
