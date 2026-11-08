// Package executor wraps os/exec with context-aware execution, tail capture
// of stdout/stderr and structured command tracing.
package executor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

// Executor runs subprocesses against an allowlist-filtered environment; it
// must be constructed via New (the zero value panics).
type Executor struct {
	env    []string
	logger *slog.Logger
}

// Option configures an Executor at construction time.
type Option func(*Executor)

// WithEnv appends vars after the allowlist-filtered parent env, so caller
// keys win on duplicates.
func WithEnv(env []string) Option {
	return func(e *Executor) { e.env = append(e.env, env...) }
}

// WithLogger injects a structured logger for command-trace output; nil
// falls back to logutil.NopLogger.
func WithLogger(l *slog.Logger) Option {
	return func(e *Executor) { e.logger = logutil.OrNop(l) }
}

// New builds an Executor with a no-op logger, then applies the options.
func New(opts ...Option) *Executor {
	e := &Executor{logger: logutil.NopLogger}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// defaultEnvAllowlist passes tooling/provider namespaces to subprocesses and
// drops the rest so unrelated tokens never reach a child; buildEnv further
// strips secret-keyed entries.
var defaultEnvAllowlist = envAllowlist{
	exact: map[string]bool{
		"PATH": true, "HOME": true, "USER": true, "LOGNAME": true, "SHELL": true,
		"LANG": true, "LANGUAGE": true, "LC_ALL": true, "LC_CTYPE": true, "LC_MESSAGES": true,
		"TERM": true, "TZ": true, "HOSTNAME": true,
		"TMPDIR": true, "TMP": true, "TEMP": true,
		"SSH_AUTH_SOCK": true, "SSH_AGENT_PID": true,
		"HTTP_PROXY": true, "HTTPS_PROXY": true, "NO_PROXY": true,
		"http_proxy": true, "https_proxy": true, "no_proxy": true,
		"SUDO_USER": true, "SUDO_UID": true, "SUDO_GID": true,
		"COLORTERM": true, "NO_COLOR": true, "FORCE_COLOR": true,
		"PAGER": true, "EDITOR": true,
		"XDG_CONFIG_HOME": true, "XDG_DATA_HOME": true,
		"XDG_CACHE_HOME": true, "XDG_RUNTIME_DIR": true,
		"DBUS_SESSION_BUS_ADDRESS": true,
		// GIT_/GITHUB_/GH_ prefixes are excluded — GITHUB_TOKEN/GH_TOKEN/
		// GIT_ASKPASS carry credentials no subprocess here needs.
		"GIT_SSH_COMMAND": true, "GIT_TERMINAL_PROMPT": true,
	},
	prefixes: []string{
		"KUBE",       // KUBECONFIG, KUBE_*
		"OC_",        // openshift-client
		"TF_",        // terraform TF_VAR_*, TF_LOG, TF_PLUGIN_*
		"TERRAFORM_", // terraform built-ins
		"PROXMOX_",   // bpg/proxmox provider + PROXMOX_VE_*
		"HELM_",
	},
}

type envAllowlist struct {
	exact    map[string]bool
	prefixes []string
}

func (a envAllowlist) allows(key string) bool {
	return a.exact[key] || slices.ContainsFunc(a.prefixes, func(p string) bool { return strings.HasPrefix(key, p) })
}

func filterParentEnv(a envAllowlist) []string {
	parent := os.Environ()
	out := make([]string, 0, len(parent))
	for _, kv := range parent {
		key, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if a.allows(key) {
			out = append(out, kv)
		}
	}
	return out
}

// buildEnv filters via defaultEnvAllowlist (dropping secret-keyed entries)
// then appends e.env, last write wins.
func (e *Executor) buildEnv() []string {
	base := dropSecretKeyed(filterParentEnv(defaultEnvAllowlist))
	if len(e.env) == 0 {
		return base
	}
	return append(base, e.env...)
}

// dropSecretKeyed strips logutil.KeyIsSecret entries: PROXMOX_/TF_ are
// allowlisted for plumbing, not for broadcasting PROXMOX_VE_PASSWORD-style
// credentials to every shellout — those need explicit WithEnv(creds.Env()).
func dropSecretKeyed(env []string) []string {
	out := env[:0]
	for _, kv := range env {
		key, _, _ := strings.Cut(kv, "=")
		if !logutil.KeyIsSecret(key) {
			out = append(out, kv)
		}
	}
	return out
}

// Result is the captured outcome of a Run-style invocation. Truncated is set
// once the ring-buffered stdout drops a line.
type Result struct {
	ExitCode  int
	Stdout    string
	Stderr    string
	Duration  time.Duration
	Truncated bool
}

// ExitError is the typed non-zero-exit error; callers errors.As it to
// inspect ExitCode without re-parsing the message. Command must be the bare
// command name; it is rendered unredacted.
type ExitError struct {
	Command  string
	ExitCode int
	Stderr   string
}

// Error truncates Stderr to 400 bytes via logutil.RedactableStderr so a
// credential-bearing diagnostic isn't leaked verbatim outside slog.
func (e *ExitError) Error() string {
	stderr := fmt.Sprint(logutil.RedactableStderr(strings.TrimSpace(e.Stderr)).Redacted())
	return fmt.Sprintf("%s failed (exit %d): %s", e.Command, e.ExitCode, stderr)
}

// Redacted omits Stderr so subprocess stderr never reaches a structured
// log sink via slog attrs.
func (e *ExitError) Redacted() any {
	return struct {
		Command  string
		ExitCode int
	}{e.Command, e.ExitCode}
}

// NewExitError returns ctx.Err() when ctx is already cancelled, so
// errors.Is(_, context.Canceled) propagates for cli/root.go's SIGINT/SIGTERM
// exit-code mapping.
func NewExitError(ctx context.Context, cmd string, code int, stderr string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return &ExitError{Command: cmd, ExitCode: code, Stderr: stderr}
}

// Run executes a command and returns its result; *Result is always
// non-nil, even on error, so callers can read ExitCode/Stderr without a
// nil guard.
func (e *Executor) Run(ctx context.Context, name string, args ...string) (*Result, error) {
	return e.run(ctx, nil, name, args...)
}

// RunWithStdin executes a command with the given string piped to its stdin.
func (e *Executor) RunWithStdin(ctx context.Context, input, name string, args ...string) (*Result, error) {
	return e.run(ctx, strings.NewReader(input), name, args...)
}

// WaitDelay bounds how long os/exec waits after ctx cancellation before it
// SIGKILLs the child and closes its I/O pipes.
const WaitDelay = 30 * time.Second

// newCmd builds the shared exec.Cmd: filtered env and SIGTERM soft-cancel
// before WaitDelay's SIGKILL escalation.
func (e *Executor) newCmd(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = e.buildEnv()
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = WaitDelay
	return cmd
}

// splitExitError folds an *exec.ExitError into result.ExitCode (returns
// nil); other errors pass through unchanged.
func splitExitError(err error, result *Result) error {
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return nil
	}
	return err
}

func (e *Executor) run(ctx context.Context, stdin io.Reader, name string, args ...string) (*Result, error) {
	start := time.Now()
	cmd := e.newCmd(ctx, name, args...)

	rout := newRingWriter(maxCapturedLines)
	rerr := newRingWriter(maxCapturedLines)
	cmd.Stdin = stdin
	cmd.Stdout = rout
	cmd.Stderr = rerr

	e.logger.Debug("exec: started", "cmd", name, "argc", len(args))

	err := cmd.Run()

	result := &Result{
		Stdout:    rout.tail(),
		Stderr:    rerr.tail(),
		Duration:  time.Since(start),
		Truncated: rout.dropped,
	}
	retErr := splitExitError(err, result)

	e.logger.Debug("exec: completed", "cmd", name, "exit", result.ExitCode, "duration", result.Duration)
	return result, retErr
}
