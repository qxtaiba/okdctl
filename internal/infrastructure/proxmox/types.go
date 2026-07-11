package proxmox

import "errors"

// Provider sentinel errors. Package-local placement is intentional:
// package-local sentinels are idiomatic Go (cf. io.EOF, fs.ErrNotExist);
// every call site wraps them in ConfigError so errors.As classification
// is unaffected.
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
