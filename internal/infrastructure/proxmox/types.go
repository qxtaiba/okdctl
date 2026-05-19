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

// VMRole is an alias of phase.NodeRole; both name the same domain concept.
type VMRole = phase.NodeRole

// Role* re-export the phase.NodeRole values for proxmox-package callers.
const (
	RoleBootstrap VMRole = phase.RoleBootstrap
	RoleMaster    VMRole = phase.RoleMaster
	RoleWorker    VMRole = phase.RoleWorker
)
