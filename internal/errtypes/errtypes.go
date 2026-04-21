// Package errtypes defines the typed error hierarchy used by okdctl.
// exitCodeFor in internal/cli/root.go maps these types to structured
// exit codes: ConfigError=2, NetworkError=3, ClusterError=4, AuthError=5.
package errtypes

import (
	"fmt"

	"github.com/qxtaiba/okdctl/internal/config"
)

// ConfigError wraps a configuration-related failure (missing file, parse
// error, validation failure). Unwrap chains to the underlying error so
// errors.Is checks on wrapped sentinels (e.g. os.ErrNotExist) still work.
//
// Error() surfaces only Msg. The inner Err is reachable via Unwrap
// (so errors.Is / errors.As walks to wrapped sentinels) but never
// string-interpolated — a credential-bearing inner error cannot leak
// past logutil.RedactHandler through the .Error() path. The same
// redaction invariant applies to NetworkError, ClusterError, AuthError.
type ConfigError struct {
	Msg string
	Err error
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("config error: %s", e.Msg)
}

func (e *ConfigError) Unwrap() error { return e.Err }

// NetworkError wraps a network-level failure (HTTP, DNS, TLS, download).
type NetworkError struct {
	Msg string
	Err error
}

func (e *NetworkError) Error() string {
	return fmt.Sprintf("network error: %s", e.Msg)
}

func (e *NetworkError) Unwrap() error { return e.Err }

// ClusterError wraps a cluster-level failure (oc/kubectl command failure,
// API unreachable, install-monitor timeout).
type ClusterError struct {
	Msg string
	Err error
}

func (e *ClusterError) Error() string {
	return fmt.Sprintf("cluster error: %s", e.Msg)
}

func (e *ClusterError) Unwrap() error { return e.Err }

// AuthError wraps an authentication or privilege-escalation failure
// (missing sudo, insecure credential file, proxmox token rejected).
type AuthError struct {
	Msg string
	Err error
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("auth error: %s", e.Msg)
}

func (e *AuthError) Unwrap() error { return e.Err }

// WrapValidation returns a *ConfigError wrapping the full validation message
// from result. Returns nil when result is nil or valid.
func WrapValidation(result *config.ValidationResult) error {
	if result == nil || result.IsValid() {
		return nil
	}
	return &ConfigError{Msg: result.Error()}
}
