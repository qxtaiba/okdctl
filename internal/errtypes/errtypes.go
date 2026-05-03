// Package errtypes defines the typed error hierarchy used by okdctl.
// exitCodeFor in internal/cli/root.go maps these types to structured
// exit codes: ConfigError=2, NetworkError=3, ClusterError=4, AuthError=5.
// Granular BSD sysexits.h sentinels (ErrConfigMissing, ErrPullSecretInvalid,
// ErrSudoMissing) refine specific failure modes within those broad categories.
package errtypes

import (
	"errors"
	"fmt"
)

// ErrConfigMissing is wrapped inside a ConfigError when the config file does
// not exist on disk. exitCodeFor maps it to 66 (EX_NOINPUT).
var ErrConfigMissing = errors.New("config file not found")

// ErrPullSecretInvalid is wrapped inside an AuthError when the pull secret
// file exists but contains invalid JSON. exitCodeFor maps it to 65 (EX_DATAERR).
var ErrPullSecretInvalid = errors.New("pull secret is not valid JSON")

// ErrSudoMissing is wrapped inside an AuthError when sudo cannot be located
// on PATH. exitCodeFor maps it to 71 (EX_OSERR).
var ErrSudoMissing = errors.New("sudo not found")

// ConfigError wraps a configuration-related failure (missing file, parse
// error, validation failure). Unwrap chains to the underlying error so
// errors.Is checks on wrapped sentinels (e.g. os.ErrNotExist) still work.
//
// Error() surfaces only Msg. The inner Err is reachable via Unwrap
// (so errors.Is / errors.As walks to wrapped sentinels) but never
// string-interpolated — a credential-bearing inner error cannot leak
// past logutil.RedactHandler through the .Error() path. The same
// redaction invariant applies to NetworkError, ClusterError, AuthError.
//
// Msg must never include credentials (passwords, tokens, secrets). Pass
// credential-bearing context only through Err so it stays in the Unwrap
// chain and out of the Error() surface string.
type ConfigError struct {
	Msg string
	Err error
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("config error: %s", e.Msg)
}

func (e *ConfigError) Unwrap() error { return e.Err }

// NetworkError wraps a network-level failure (HTTP, DNS, TLS, download).
// Msg must never include credentials; see ConfigError for the full contract.
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
// Msg must never include credentials; see ConfigError for the full contract.
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
// Msg must never include credentials; see ConfigError for the full contract.
type AuthError struct {
	Msg string
	Err error
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("auth error: %s", e.Msg)
}

func (e *AuthError) Unwrap() error { return e.Err }
