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
	Status    VMState
	IPAddress string
}

// VMRole classifies a VM's cluster role; aliased to phase.NodeRole so the
// two enums never drift. The Role* constants below are package-local
// re-exports of the canonical phase constants.
type VMRole = phase.NodeRole

// Role* mirror phase's canonical values so existing call sites stay terse.
const (
	RoleBootstrap = phase.RoleBootstrap
	RoleMaster    = phase.RoleMaster
	RoleWorker    = phase.RoleWorker
)

// VMState aliases phase.VMState so iso_cleanup (phase) and proxmox share
// a single canonical lifecycle enum. The State* constants below mirror the
// canonical phase values.
type VMState = phase.VMState

// State* mirror phase's canonical values so existing call sites stay terse.
const (
	StateRunning  = phase.StateRunning
	StateStopped  = phase.StateStopped
	StateCreating = phase.StateCreating
	StateDeleting = phase.StateDeleting
	StateUnknown  = phase.StateUnknown
)
