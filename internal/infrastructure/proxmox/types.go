package proxmox

import "errors"

var (
	ErrNotConnected           = errors.New("not connected to provider")
	ErrTerraformNotConfigured = errors.New("terraform not configured")
)

type ProvisionOptions struct {
	AutoApprove  bool
	ProjectRoot  string
	TerraformEnv string
}

type ProvisionResult struct {
	VMs             []VMStatus
	BootstrapIP     string
	ControlPlaneIPs []string
	WorkerIPs       []string
	APIServerIP     string
}

type VMStatus struct {
	ID   string
	Name string
	// Role is one of: bootstrap, master, worker.
	Role string
	// Status is one of: running, stopped, creating, deleting, unknown.
	Status    string
	IPAddress string
}

type VMRole string

const (
	RoleBootstrap VMRole = "bootstrap"
	RoleMaster    VMRole = "master"
	RoleWorker    VMRole = "worker"
)

type VMState string

const (
	StateRunning  VMState = "running"
	StateStopped  VMState = "stopped"
	StateCreating VMState = "creating"
	StateDeleting VMState = "deleting"
	StateUnknown  VMState = "unknown"
)
