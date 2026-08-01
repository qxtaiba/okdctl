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

// HintAppender is implemented by error types that can carry an extra
// next-step hint without changing their concrete type — and therefore
// without changing exitCodeFor's exit-code classification. The hint is
// stored structurally (a separate field), not concatenated onto Msg, so
// render.ErrorSummary reads it back via Describe rather than re-splitting
// Error() on "; ". All five categories implement it uniformly;
// terraform.Executor.WithLockHint is the only caller today.
type HintAppender interface {
	WithHint(hint string) error
}

// appendHint combines an existing hint with a new one; multiple hints
// accumulate joined by "; ". The empty existing case returns hint verbatim.
func appendHint(existing, hint string) string {
	if existing == "" {
		return hint
	}
	return existing + "; " + hint
}

// withHintSuffix appends "; <hint>" to s for the Error() log surface, leaving
// s untouched when hint is empty. The hint lives in a struct field, not Msg,
// so this suffix is the only place display text and hint are concatenated.
func withHintSuffix(s, hint string) string {
	if hint == "" {
		return s
	}
	return s + "; " + hint
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
	Msg  string
	Err  error
	hint string
}

func (e *ConfigError) Error() string {
	return withHintSuffix(fmt.Sprintf("config error: %s", e.Msg), e.hint)
}

func (e *ConfigError) Unwrap() error { return e.Err }

// WithHint returns a copy of e carrying hint as structured next-step text;
// e itself is left unmodified and Msg is not touched. Implements HintAppender.
func (e *ConfigError) WithHint(hint string) error {
	c := *e
	c.hint = appendHint(e.hint, hint)
	return &c
}

// NetworkError wraps a network-level failure (HTTP, DNS, TLS, download).
// Msg must never include credentials; see ConfigError for the full contract.
type NetworkError struct {
	Msg  string
	Err  error
	hint string
}

func (e *NetworkError) Error() string {
	return withHintSuffix(fmt.Sprintf("network error: %s", e.Msg), e.hint)
}

func (e *NetworkError) Unwrap() error { return e.Err }

// WithHint returns a copy of e carrying hint as structured next-step text;
// e itself is left unmodified and Msg is not touched. Implements HintAppender.
func (e *NetworkError) WithHint(hint string) error {
	c := *e
	c.hint = appendHint(e.hint, hint)
	return &c
}

// ClusterError wraps a cluster-level failure (oc/kubectl command failure,
// API unreachable, install-monitor timeout).
// Msg must never include credentials; see ConfigError for the full contract.
type ClusterError struct {
	Msg  string
	Err  error
	hint string
}

func (e *ClusterError) Error() string {
	return withHintSuffix(fmt.Sprintf("cluster error: %s", e.Msg), e.hint)
}

func (e *ClusterError) Unwrap() error { return e.Err }

// WithHint returns a copy of e carrying hint as structured next-step text;
// e itself is left unmodified and Msg is not touched. Implements HintAppender.
func (e *ClusterError) WithHint(hint string) error {
	c := *e
	c.hint = appendHint(e.hint, hint)
	return &c
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
	hint string
}

func (e *AuthError) Error() string {
	base := fmt.Sprintf("auth error: %s", e.Msg)
	if e.Path != "" {
		base = fmt.Sprintf("auth error: %s (path: %s)", e.Msg, e.Path)
	}
	return withHintSuffix(base, e.hint)
}

func (e *AuthError) Unwrap() error { return e.Err }

// WithHint returns a copy of e carrying hint as structured next-step text;
// e itself is left unmodified and Msg is not touched. Implements HintAppender.
func (e *AuthError) WithHint(hint string) error {
	c := *e
	c.hint = appendHint(e.hint, hint)
	return &c
}

// UsageError wraps a command-line flag-parse failure. exitCodeFor maps it to
// 64 (EX_USAGE per BSD sysexits.h). SetFlagErrorFunc returns this instead of
// calling os.Exit so Execute's deferred logFileCloser.Close() runs first.
// Msg must never include credentials; see ConfigError for the full contract.
type UsageError struct {
	Msg  string
	Err  error
	hint string
}

func (e *UsageError) Error() string {
	return withHintSuffix(fmt.Sprintf("usage error: %s", e.Msg), e.hint)
}

func (e *UsageError) Unwrap() error { return e.Err }

// WithHint returns a copy of e carrying hint as structured next-step text;
// e itself is left unmodified and Msg is not touched. Implements HintAppender.
func (e *UsageError) WithHint(hint string) error {
	c := *e
	c.hint = appendHint(e.hint, hint)
	return &c
}

// Kind enumerates the top-level errtypes categories for display and
// exit-code classification. KindUnknown covers untyped errors (exit 1).
type Kind int

// Kind values, in exitCodeFor's documented precedence order.
const (
	KindUnknown Kind = iota
	KindConfig
	KindNetwork
	KindCluster
	KindAuth
	KindUsage
)

// Label returns the human category name used as an error-box headline chip.
func (k Kind) Label() string {
	switch k {
	case KindConfig:
		return "config error"
	case KindNetwork:
		return "network error"
	case KindCluster:
		return "cluster error"
	case KindAuth:
		return "auth error"
	case KindUsage:
		return "usage error"
	default:
		return "error"
	}
}

// ExitCode returns the BSD-sysexits code for the category tier only:
// Config=2, Network=3, Cluster=4, Auth=5, Usage=64, Unknown=1. It does NOT
// encode the sentinel tier (ErrConfigMissing=66, ErrPullSecretInvalid=65,
// ErrSudoMissing=71); exitCodeFor checks those sentinels ahead of the
// category tier.
func (k Kind) ExitCode() int {
	switch k {
	case KindConfig:
		return 2
	case KindNetwork:
		return 3
	case KindCluster:
		return 4
	case KindAuth:
		return 5
	case KindUsage:
		return 64
	default:
		return 1
	}
}

// Display is the render-facing decomposition of a typed error: the category
// Kind, the human Message with no category prefix and no hint suffix, and the
// optional next-step Hint. render.ErrorSummary consumes it to lay out the
// error box without re-parsing Error() or string-splitting on "; ".
type Display struct {
	Kind    Kind
	Message string
	Hint    string
}

// Describe decomposes err into its Display parts, walking the Unwrap chain in
// exitCodeFor's precedence order (Config > Network > Cluster > Auth > Usage).
// ok is false when err is not (and does not wrap) a known category, in which
// case the caller should render Kind.Label() ("error") with err.Error().
func Describe(err error) (Display, bool) {
	var cfg *ConfigError
	if errors.As(err, &cfg) {
		return Display{Kind: KindConfig, Message: cfg.Msg, Hint: cfg.hint}, true
	}
	var net *NetworkError
	if errors.As(err, &net) {
		return Display{Kind: KindNetwork, Message: net.Msg, Hint: net.hint}, true
	}
	var cluster *ClusterError
	if errors.As(err, &cluster) {
		return Display{Kind: KindCluster, Message: cluster.Msg, Hint: cluster.hint}, true
	}
	var auth *AuthError
	if errors.As(err, &auth) {
		msg := auth.Msg
		if auth.Path != "" {
			msg = fmt.Sprintf("%s (path: %s)", auth.Msg, auth.Path)
		}
		return Display{Kind: KindAuth, Message: msg, Hint: auth.hint}, true
	}
	var usage *UsageError
	if errors.As(err, &usage) {
		return Display{Kind: KindUsage, Message: usage.Msg, Hint: usage.hint}, true
	}
	return Display{}, false
}

// Classify reports the Kind of err, the single classifier the render layer
// and exitCodeFor consume in place of hand-written errors.As ladders. It
// shares Describe's precedence order; ok is false for untyped errors
// (KindUnknown). It intentionally does not inspect the sentinel tier.
func Classify(err error) (Kind, bool) {
	d, ok := Describe(err)
	return d.Kind, ok
}
