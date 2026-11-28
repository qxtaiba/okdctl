// Package executor wraps os/exec with an env allowlist, graceful SIGTERM
// cancellation and streamed or tail-captured child output.
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

// defaultEnvAllowlist is what a child (openshift-install, oc, ping) needs
// from the parent environment; everything else, tokens included, stays out.
var defaultEnvAllowlist = envAllowlist{
	exact: map[string]bool{
		"PATH": true, "HOME": true, "USER": true,
		"LANG": true, "LANGUAGE": true, "LC_ALL": true, "LC_CTYPE": true, "LC_MESSAGES": true,
		"TERM": true, "TZ": true,
		"TMPDIR": true, "TMP": true, "TEMP": true,
		"SSH_AUTH_SOCK": true,
		"HTTP_PROXY":    true, "HTTPS_PROXY": true, "NO_PROXY": true,
		"http_proxy": true, "https_proxy": true, "no_proxy": true,
		"XDG_CONFIG_HOME": true, "XDG_DATA_HOME": true,
		"XDG_CACHE_HOME": true, "XDG_RUNTIME_DIR": true,
	},
	prefixes: []string{"KUBE"}, // KUBECONFIG, KUBE_*
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

// dropSecretKeyed strips logutil.KeyIsSecret entries the KUBE prefix would
// let through; a credential a child needs arrives by WithEnv.
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

// Run executes a command and returns its result; *Result is always
// non-nil, even on error, so callers can read ExitCode/Stderr without a
// nil guard. A child cut short by ctx returns ctx's error, not its exit.
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
	p, err := e.start(ctx, stdin, nil, name, args...)
	if err != nil {
		return &Result{}, err
	}
	return p.Wait()
}
