package proxmox

import (
	"errors"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
)

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
	Status    phase.VMState
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
