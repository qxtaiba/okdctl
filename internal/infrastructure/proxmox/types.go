package proxmox

import "errors"

// Provider sentinel errors.
var (
	ErrNotConnected           = errors.New("not connected to provider")
	ErrTerraformNotConfigured = errors.New("terraform not configured")
)

// ProvisionOptions configures a single Provision call.
type ProvisionOptions struct {
	AutoApprove  bool
	ProjectRoot  string
	TerraformEnv string
}

// ProvisionResult summarizes the VMs produced by a Provision call.
type ProvisionResult struct {
	VMs             []VMStatus
	BootstrapIP     string
	ControlPlaneIPs []string
	WorkerIPs       []string
	APIServerIP     string
}

// VMStatus describes the current state of a single Proxmox VM.
type VMStatus struct {
	ID        string
	Name      string
	Role      VMRole
	Status    VMState
	IPAddress string
}

// VMRole classifies a VM's cluster role. String values match what
// openshift-install, HAProxy templates, and ignition URLs expect verbatim.
type VMRole string

// Role* are the valid VMRole values.
const (
	RoleBootstrap VMRole = "bootstrap"
	RoleMaster    VMRole = "master"
	RoleWorker    VMRole = "worker"
)

// VMState classifies a Proxmox VM's lifecycle state. String values match the
// "status" field in `pvesh get /nodes/<n>/qemu` output verbatim.
type VMState string

// State* are the valid VMState values.
const (
	StateRunning  VMState = "running"
	StateStopped  VMState = "stopped"
	StateCreating VMState = "creating"
	StateDeleting VMState = "deleting"
	StateUnknown  VMState = "unknown"
)
