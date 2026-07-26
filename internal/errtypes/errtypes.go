// Package errtypes defines the typed error hierarchy used by okdctl.
// exitCodeFor in internal/cli/root.go maps each type to a structured exit code:
// ConfigError=2, NetworkError=3, ClusterError=4, AuthError=5, UsageError=64.
//
// The taxonomy intentionally has no transient/recoverable concept. Failures
// such as a VIP certificate not yet rotated or a CSR still pending are
// classified as ClusterError (exit 4) until a retry-aware consumer exists
// that needs to distinguish them from permanently-degraded states. Introducing
// a TransientError type is deferred until that consumer lands. Until then,
// three sites carry ad-hoc retry classification that TransientError will
// consolidate: infrastructure/proxmox/proxmox.go::initIsRetryable,
// addon/helpers.go::addonIsRetryable, download/retry.go::isRetryable.
package errtypes

import (
	"context"
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

// ErrWaitTimeout marks a poll-loop timeout raised by system.WaitFor's own
// opts.Timeout, as opposed to a deadline set on the caller's context. It
// wraps context.DeadlineExceeded so existing errors.Is matchers keep
// working; match ErrWaitTimeout to single out poll timeouts specifically.
var ErrWaitTimeout = fmt.Errorf("wait timeout: %w", context.DeadlineExceeded)

// HintAppender is implemented by error types whose Msg can be enriched with
// extra diagnostic text without changing their concrete type — and
// therefore without changing exitCodeFor's exit-code classification.
// terraform.Executor.WithLockHint is the only caller today.
type HintAppender interface {
	WithHint(hint string) error
}

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
// Msg must never include credentials. Construction-site scanning enforces
// password and api_key fragments via TestMsgFieldNoCredentialInterpolation;
// the broader "tokens / secrets" axis is enforced only by reviewer
// discipline because those substrings collide with benign descriptive
// words ("pull secret", "csrf token"). Pass credential-bearing context only
// through Err so it stays in the Unwrap chain and out of Error().
type ConfigError struct {
	Msg string
	Err error
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("config error: %s", e.Msg)
}

func (e *ConfigError) Unwrap() error { return e.Err }

// WithHint returns a copy of e with hint appended to Msg; e itself is left
// unmodified. Implements HintAppender.
func (e *ConfigError) WithHint(hint string) error {
	return &ConfigError{Msg: e.Msg + "; " + hint, Err: e.Err}
}

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

// WithHint returns a copy of e with hint appended to Msg; e itself is left
// unmodified. Implements HintAppender.
func (e *ClusterError) WithHint(hint string) error {
	return &ClusterError{Msg: e.Msg + "; " + hint, Err: e.Err}
}

// AuthError wraps an authentication or privilege-escalation failure
// (missing sudo, insecure credential file, proxmox token rejected).
// Msg must never include credentials; see ConfigError for the full contract.
// Path carries a filesystem path when the failure originates from a
// permission check; it is structured so RedactHandler can apply uniform
// path policy without re-parsing Msg.
type AuthError struct {
	Msg  string
	Path string
	Err  error
}

func (e *AuthError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("auth error: %s (path: %s)", e.Msg, e.Path)
	}
	return fmt.Sprintf("auth error: %s", e.Msg)
}

func (e *AuthError) Unwrap() error { return e.Err }

// UsageError wraps a command-line flag-parse failure. exitCodeFor maps it to
// 64 (EX_USAGE per BSD sysexits.h). SetFlagErrorFunc returns this instead of
// calling os.Exit so Execute's deferred logFileCloser.Close() runs first.
// Msg must never include credentials; see ConfigError for the full contract.
type UsageError struct {
	Msg string
	Err error
}

func (e *UsageError) Error() string {
	return fmt.Sprintf("usage error: %s", e.Msg)
}

func (e *UsageError) Unwrap() error { return e.Err }
