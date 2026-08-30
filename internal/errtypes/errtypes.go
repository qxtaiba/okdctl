// Package errtypes defines okdctl's typed error hierarchy; exitCodeFor
// (cli/root.go) maps each type to an exit code (Config=2, Network=3, Cluster=4, Auth=5, Usage=64).
package errtypes

import (
	"context"
	"errors"
	"fmt"
)

// ErrConfigMissing is wrapped in ConfigError when the config file is missing;
// exitCodeFor maps it to 66 (EX_NOINPUT).
var ErrConfigMissing = errors.New("config file not found")

// ErrPullSecretInvalid is wrapped in AuthError when the pull secret file has
// invalid JSON; exitCodeFor maps it to 65 (EX_DATAERR).
var ErrPullSecretInvalid = errors.New("pull secret is not valid JSON")

// ErrSudoMissing is wrapped in AuthError when sudo isn't found on PATH;
// exitCodeFor maps it to 71 (EX_OSERR).
var ErrSudoMissing = errors.New("sudo not found")

// ErrWaitTimeout marks a poll-loop timeout from WaitFor's own opts.Timeout
// (not the caller's ctx deadline); it wraps context.DeadlineExceeded so errors.Is still matches.
var ErrWaitTimeout = fmt.Errorf("wait timeout: %w", context.DeadlineExceeded)

// HintAppender lets an error type carry a next-step hint without changing its
// type or exit code; the hint is a separate field so Describe reads it back structurally.
type HintAppender interface {
	WithHint(hint string) error
}

func appendHint(existing, hint string) string {
	if existing == "" {
		return hint
	}
	return existing + "; " + hint
}

// withHintSuffix appends "; <hint>" to s for Error(), the only place hint and
// message text are concatenated.
func withHintSuffix(s, hint string) string {
	if hint == "" {
		return s
	}
	return s + "; " + hint
}

// ConfigError wraps a configuration failure. Msg must never contain
// credentials — Error() surfaces only Msg, never Err; pass credential-bearing
// context through Err only. Same contract applies to NetworkError, ClusterError, AuthError.
type ConfigError struct {
	Msg  string
	Err  error
	hint string
}

func (e *ConfigError) Error() string {
	return withHintSuffix(fmt.Sprintf("config error: %s", e.Msg), e.hint)
}

func (e *ConfigError) Unwrap() error { return e.Err }

// WithHint returns a copy of e carrying hint; e is left unmodified. Implements HintAppender.
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

// WithHint returns a copy of e carrying hint; e is left unmodified. Implements HintAppender.
func (e *NetworkError) WithHint(hint string) error {
	c := *e
	c.hint = appendHint(e.hint, hint)
	return &c
}

// ClusterError wraps a cluster-level failure (oc/kubectl failure, API
// unreachable, install-monitor timeout); see ConfigError for the Msg/credential contract.
type ClusterError struct {
	Msg  string
	Err  error
	hint string
}

func (e *ClusterError) Error() string {
	return withHintSuffix(fmt.Sprintf("cluster error: %s", e.Msg), e.hint)
}

func (e *ClusterError) Unwrap() error { return e.Err }

// WithHint returns a copy of e carrying hint; e is left unmodified. Implements HintAppender.
func (e *ClusterError) WithHint(hint string) error {
	c := *e
	c.hint = appendHint(e.hint, hint)
	return &c
}

// AuthError wraps an auth/privilege-escalation failure; Msg follows
// ConfigError's credential contract, and Path stays separate so RedactHandler
// applies uniform policy.
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

// WithHint returns a copy of e carrying hint; e is left unmodified. Implements HintAppender.
func (e *AuthError) WithHint(hint string) error {
	c := *e
	c.hint = appendHint(e.hint, hint)
	return &c
}

// UsageError wraps a command-line flag-parse failure (exitCodeFor: 64, EX_USAGE).
// SetFlagErrorFunc returns this instead of calling os.Exit so Execute's
// deferred logFileCloser.Close() still runs.
type UsageError struct {
	Msg  string
	Err  error
	hint string
}

func (e *UsageError) Error() string {
	return withHintSuffix(fmt.Sprintf("usage error: %s", e.Msg), e.hint)
}

func (e *UsageError) Unwrap() error { return e.Err }

// WithHint returns a copy of e carrying hint; e is left unmodified. Implements HintAppender.
func (e *UsageError) WithHint(hint string) error {
	c := *e
	c.hint = appendHint(e.hint, hint)
	return &c
}

// Kind enumerates the errtypes categories for display and exit-code
// classification; KindUnknown covers untyped errors (exit 1).
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

// ExitCode returns the BSD-sysexits code for the category tier (Config=2,
// Network=3, Cluster=4, Auth=5, Usage=64, Unknown=1). It excludes the sentinel
// tier (66/65/71); exitCodeFor checks those first.
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

// Display is the render-facing decomposition of a typed error into Kind, a
// prefix/suffix-free Message, and an optional Hint. render.ErrorSummary
// consumes it directly, without re-parsing Error() or splitting on "; ".
type Display struct {
	Kind    Kind
	Message string
	Hint    string
}

// Describe decomposes err into Display parts, walking the Unwrap chain in
// exitCodeFor's precedence order (Config > Network > Cluster > Auth > Usage).
// ok is false for an unknown category; the caller should then render
// Kind.Label() ("error") with err.Error().
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

// Classify reports the Kind of err using Describe's precedence order; ok is
// false for untyped errors.
func Classify(err error) (Kind, bool) {
	d, ok := Describe(err)
	return d.Kind, ok
}
