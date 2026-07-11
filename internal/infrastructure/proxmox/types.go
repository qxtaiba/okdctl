package proxmox

import (
	"errors"

	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

// Provider sentinel errors. Package-local placement is intentional
// (err:d6b325cb): package-local sentinels are idiomatic Go (cf. io.EOF,
// fs.ErrNotExist); every call site wraps them in ConfigError so
// errors.As classification is unaffected.
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
	Role      nodetypes.NodeRole
	Status    nodetypes.VMState
	IPAddress string
}
