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

// VMRole classifies a VM's cluster role (bootstrap/master/worker).
type VMRole string

// VM role values.
const (
	RoleBootstrap VMRole = "bootstrap"
	RoleMaster    VMRole = "master"
	RoleWorker    VMRole = "worker"
)

// VMState classifies a VM's lifecycle state.
type VMState string

// VM lifecycle state values.
const (
	StateRunning  VMState = "running"
	StateStopped  VMState = "stopped"
	StateCreating VMState = "creating"
	StateDeleting VMState = "deleting"
	StateUnknown  VMState = "unknown"
)
