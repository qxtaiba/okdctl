// Package proxmox implements the Proxmox VE provider.
package proxmox

import "errors"

// Sentinel errors for provider operations.
var (
	// ErrNotConnected is returned when an operation requires an active connection.
	ErrNotConnected = errors.New("not connected to provider")

	// ErrTerraformNotConfigured is returned when Terraform operations are attempted without setup.
	ErrTerraformNotConfigured = errors.New("terraform not configured")
)

// ProvisionOptions contains options for the Provision operation.
type ProvisionOptions struct {
	// AutoApprove if true, skips confirmation prompts.
	AutoApprove bool

	// ProjectRoot is the root directory of the project (for Terraform).
	ProjectRoot string

	// TerraformEnv is the Terraform environment name.
	TerraformEnv string
}

// ProvisionResult contains the result of a Provision operation.
type ProvisionResult struct {
	// VMs is the list of created VMs.
	VMs []VMStatus

	// BootstrapIP is the IP of the bootstrap node (if applicable).
	BootstrapIP string

	// ControlPlaneIPs are the IPs of control plane nodes.
	ControlPlaneIPs []string

	// WorkerIPs are the IPs of worker nodes.
	WorkerIPs []string

	// APIServerIP is the VIP for the API server (if using VIP).
	APIServerIP string
}

// VMStatus represents the status of a virtual machine.
type VMStatus struct {
	// ID is the provider-specific VM identifier.
	ID string

	// Name is the VM name.
	Name string

	// Role is the node role (bootstrap, master, worker).
	Role string

	// Status is the VM status (running, stopped, creating, etc.).
	Status string

	// IPAddress is the primary IP address.
	IPAddress string
}

// VMRole represents the role of a VM in the cluster.
type VMRole string

const (
	RoleBootstrap VMRole = "bootstrap"
	RoleMaster    VMRole = "master"
	RoleWorker    VMRole = "worker"
)

// VMState represents the state of a VM.
type VMState string

const (
	StateRunning  VMState = "running"
	StateStopped  VMState = "stopped"
	StateCreating VMState = "creating"
	StateDeleting VMState = "deleting"
	StateUnknown  VMState = "unknown"
)
