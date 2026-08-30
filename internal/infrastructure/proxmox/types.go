package proxmox

import "errors"

// Provider sentinel errors; every call site wraps them in ConfigError without breaking errors.As.
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
