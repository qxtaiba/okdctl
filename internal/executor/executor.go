// Package executor wraps os/exec with context-aware execution, output
// capture, and structured logging for setup/install/postinstall phases.
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

// Executor wraps os/exec with context-aware execution, output capture, and
// structured logging; must be constructed via New (the zero value panics).
type Executor struct {
	workDir      string
	env          []string
	stdout       io.Writer
	stderr       io.Writer
	inheritEnv   bool
	cancelSignal syscall.Signal
	logger       *slog.Logger
}

// Option configures an Executor at construction time.
type Option func(*Executor)

// WithWorkDir sets the working directory for commands run by the Executor.
func WithWorkDir(dir string) Option {
	return func(e *Executor) { e.workDir = dir }
}

// WithStdout redirects streamed command output from the default os.Stdout.
func WithStdout(w io.Writer) Option {
	return func(e *Executor) { e.stdout = w }
}

// WithStderr redirects streamed command error output from the default os.Stderr.
func WithStderr(w io.Writer) Option {
	return func(e *Executor) { e.stderr = w }
}

// WithEnv appends vars after the allowlist-filtered (or fully inherited)
// parent env, so caller keys win on duplicates.
func WithEnv(env []string) Option {
	return func(e *Executor) { e.env = append(e.env, env...) }
}

// WithLogger injects a structured logger for command-trace output; nil
// falls back to logutil.NopLogger.
func WithLogger(l *slog.Logger) Option {
	return func(e *Executor) { e.logger = logutil.OrNop(l) }
}

// WithInheritedEnv disables the allowlist and passes the parent's full
// environment to subprocesses; use sparingly, only for a tool or test that
// needs a non-allowlisted variable.
func WithInheritedEnv() Option {
	return func(e *Executor) { e.inheritEnv = true }
}

// WithCancelSignal overrides cmd.Cancel's signal (default SIGTERM, giving
// guarded subprocesses time to flush before the 30s SIGKILL escalation).
// Use SIGINT only for a terraform-backed Executor — its documented
// soft-cancel releases the state lock before exit.
func WithCancelSignal(sig syscall.Signal) Option {
	return func(e *Executor) { e.cancelSignal = sig }
}

// New builds an Executor with defaults wired to os.Stdout/os.Stderr and a
// no-op logger, then applies the provided options.
func New(opts ...Option) *Executor {
	e := &Executor{
		stdout:       os.Stdout,
		stderr:       os.Stderr,
		cancelSignal: syscall.SIGTERM,
		logger:       logutil.NopLogger,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// DefaultEnvAllowlist is the env filter shared by Executor subprocesses and
// the sudo re-exec in cli/elevation.go: it passes tooling/provider
// namespaces and drops the rest so unrelated tokens don't reach privileged
// processes. Executor subprocesses further drop secret-keyed entries
// (buildEnv); only direct FilterParentEnv callers (self re-exec) get
// allowlisted credentials like PROXMOX_VE_PASSWORD.
var DefaultEnvAllowlist = EnvAllowlist{
	Exact: map[string]bool{
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
	Prefixes: []string{
		"KUBE",       // KUBECONFIG, KUBE_*
		"OC_",        // openshift-client
		"TF_",        // terraform TF_VAR_*, TF_LOG, TF_PLUGIN_*
		"TERRAFORM_", // terraform built-ins
		"PROXMOX_",   // bpg/proxmox provider + PROXMOX_VE_*
		"HELM_",
	},
}

// EnvAllowlist is a dual exact+prefix filter for environment variables,
// exported so callers like cli/elevation.go can reuse it.
type EnvAllowlist struct {
	Exact    map[string]bool
	Prefixes []string
}

func (a EnvAllowlist) allows(key string) bool {
	return a.Exact[key] || slices.ContainsFunc(a.Prefixes, func(p string) bool { return strings.HasPrefix(key, p) })
}

// FilterParentEnv returns os.Environ() entries whose keys pass the
// allowlist, exported for reuse by the sudo re-exec path in cli/elevation.go.
func FilterParentEnv(a EnvAllowlist) []string {
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

// buildEnv filters via DefaultEnvAllowlist (dropping secret-keyed entries)
// then appends e.env, last write wins; inherit mode passes os.Environ()
// through unchanged.
func (e *Executor) buildEnv() []string {
	if e.inheritEnv {
		if len(e.env) == 0 {
			return nil // nil means os.Environ() by os/exec contract
		}
		return append(os.Environ(), e.env...)
	}
	base := dropSecretKeyed(FilterParentEnv(DefaultEnvAllowlist))
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
// once ring-buffered stdout drops a line (Run/RunStreamed) or buffered
// stdout exceeds the byte cap (RunOutput); machine-parsing callers must
// check it via RunOutput/RunOutputChecked before unmarshalling.
type Result struct {
	ExitCode  int
	Stdout    string
	Stderr    string
	Duration  time.Duration
	Truncated bool
}

// ExitError is the typed error RunChecked/RunWithStdinChecked return on
// non-zero exit; callers can errors.As it to inspect ExitCode without
// re-parsing the message.
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

// newCmd builds the shared exec.Cmd: workDir, filtered env, and soft-cancel
// via e.cancelSignal before WaitDelay's SIGKILL escalation.
func (e *Executor) newCmd(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	if e.workDir != "" {
		cmd.Dir = e.workDir
	}
	cmd.Env = e.buildEnv()
	cmd.Cancel = func() error { return cmd.Process.Signal(e.cancelSignal) }
	cmd.WaitDelay = 30 * time.Second
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

// RunStreamed pipes stdout/stderr live to e.stdout/e.stderr while retaining
// the last maxCapturedLines in Result for error reporting; *Result is
// always non-nil.
func (e *Executor) RunStreamed(ctx context.Context, name string, args ...string) (*Result, error) {
	start := time.Now()
	cmd := e.newCmd(ctx, name, args...)

	rout := newRingWriter(maxCapturedLines)
	rerr := newRingWriter(maxCapturedLines)
	cmd.Stdout = io.MultiWriter(e.stdout, rout)
	cmd.Stderr = io.MultiWriter(e.stderr, rerr)

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

// RunStreamedChecked is RunStreamed with RunChecked semantics: non-zero
// exit returns an *ExitError carrying the tail of stderr.
func (e *Executor) RunStreamedChecked(ctx context.Context, name string, args ...string) (*Result, error) {
	result, err := e.RunStreamed(ctx, name, args...)
	if err != nil {
		return result, err
	}
	if result.ExitCode != 0 {
		return result, NewExitError(ctx, name, result.ExitCode, result.Stderr)
	}
	return result, nil
}

// StartStreamed starts name with args, piping stdout/stderr live, and
// returns immediately with a channel that receives cmd.Wait's result. kill
// is a deliberate no-op — cmd.Cancel already handles ctx cancellation; it
// exists only for API symmetry with callers expecting an explicit kill func.
func (e *Executor) StartStreamed(ctx context.Context, name string, args ...string) (done <-chan error, kill func(), err error) {
	cmd := e.newCmd(ctx, name, args...)
	cmd.Stdout = e.stdout
	cmd.Stderr = e.stderr

	e.logger.Debug("exec: started", "cmd", name, "argc", len(args))
	if startErr := cmd.Start(); startErr != nil {
		return nil, func() {}, startErr
	}

	doneCh := make(chan error, 1)
	go func() {
		defer close(doneCh)
		doneCh <- cmd.Wait()
	}()
	return doneCh, func() {}, nil
}

// RunInteractive executes a command wired to the current process's stdin and
// the Executor's stdout/stderr for user-facing prompts.
func (e *Executor) RunInteractive(ctx context.Context, name string, args ...string) error {
	start := time.Now()
	cmd := e.newCmd(ctx, name, args...)

	cmd.Stdin = os.Stdin
	cmd.Stdout = e.stdout
	cmd.Stderr = e.stderr

	e.logger.Debug("exec: started", "cmd", name, "argc", len(args))

	err := cmd.Run()
	e.logger.Debug("exec: completed", "cmd", name, "exit", exitCodeOf(err), "duration", time.Since(start))
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
	}
	return err
}

// exitCodeOf maps a cmd.Run error to its exit code for logging: 0 for nil,
// the exit status for *exec.ExitError, -1 otherwise.
func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

// RunChecked executes a command and errors on launch failure or non-zero
// exit, returned as *ExitError for errors.As inspection. Use Run directly
// when non-zero exit is acceptable (probing, cleanup).
func (e *Executor) RunChecked(ctx context.Context, name string, args ...string) (*Result, error) {
	result, err := e.Run(ctx, name, args...)
	if err != nil {
		return result, err
	}
	if result.ExitCode != 0 {
		return result, NewExitError(ctx, name, result.ExitCode, result.Stderr)
	}
	return result, nil
}

// RunDiscard executes a command with stdout fully discarded (not
// ring-buffered) while stderr stays ring-capped at maxCapturedLines; use
// for high-volume stdout where only success/failure and stderr matter.
// *Result is always non-nil; Result.Stdout is always empty.
func (e *Executor) RunDiscard(ctx context.Context, name string, args ...string) (*Result, error) {
	start := time.Now()
	cmd := e.newCmd(ctx, name, args...)

	rerr := newRingWriter(maxCapturedLines)
	cmd.Stdout = io.Discard
	cmd.Stderr = rerr

	e.logger.Debug("exec: started", "cmd", name, "argc", len(args))
	err := cmd.Run()

	result := &Result{
		Stderr:   rerr.tail(),
		Duration: time.Since(start),
	}
	retErr := splitExitError(err, result)

	e.logger.Debug("exec: completed", "cmd", name, "exit", result.ExitCode, "duration", result.Duration)
	return result, retErr
}

// RunDiscardChecked is RunDiscard with RunChecked semantics: non-zero exit
// returns an *ExitError carrying the stderr tail.
func (e *Executor) RunDiscardChecked(ctx context.Context, name string, args ...string) (*Result, error) {
	result, err := e.RunDiscard(ctx, name, args...)
	if err != nil {
		return result, err
	}
	if result.ExitCode != 0 {
		return result, NewExitError(ctx, name, result.ExitCode, result.Stderr)
	}
	return result, nil
}

// runOutputMaxBytes is the default stdout cap for RunOutput when limit=0.
const runOutputMaxBytes = 4 * 1024 * 1024

// RunOutput executes a command and returns full stdout up to limit bytes (0
// defaults to runOutputMaxBytes), fully buffered rather than ring-truncated
// so it's safe for machine-parsing large JSON payloads. On overrun,
// Result.Truncated is true and Stdout holds the capped prefix; *Result is
// always non-nil.
func (e *Executor) RunOutput(ctx context.Context, limit int, name string, args ...string) (*Result, error) {
	return e.runOutput(ctx, nil, limit, name, args...)
}

// RunOutputChecked is RunOutput with RunChecked semantics: non-zero exit
// returns an *ExitError carrying the stderr tail.
func (e *Executor) RunOutputChecked(ctx context.Context, limit int, name string, args ...string) (*Result, error) {
	result, err := e.RunOutput(ctx, limit, name, args...)
	if err != nil {
		return result, err
	}
	if result.ExitCode != 0 {
		return result, NewExitError(ctx, name, result.ExitCode, result.Stderr)
	}
	return result, nil
}

func (e *Executor) runOutput(ctx context.Context, stdin io.Reader, limit int, name string, args ...string) (*Result, error) {
	if limit <= 0 {
		limit = runOutputMaxBytes
	}
	start := time.Now()
	cmd := e.newCmd(ctx, name, args...)

	rerr := newRingWriter(maxCapturedLines)
	cmd.Stdin = stdin
	cmd.Stderr = rerr

	stdoutPipe, pipeErr := cmd.StdoutPipe()
	if pipeErr != nil {
		return &Result{Duration: time.Since(start)}, pipeErr
	}

	e.logger.Debug("exec: started", "cmd", name, "argc", len(args))

	if err := cmd.Start(); err != nil {
		return &Result{Duration: time.Since(start)}, err
	}

	// Read limit+1 to distinguish exact-limit from overrun, then drain the
	// rest — otherwise a child writing past the pipe buffer would block
	// forever and Wait would never return.
	out, readErr := io.ReadAll(io.LimitReader(stdoutPipe, int64(limit)+1))
	_, _ = io.Copy(io.Discard, stdoutPipe)
	waitErr := cmd.Wait()

	truncated := len(out) > limit
	if truncated {
		out = out[:limit]
	}

	result := &Result{
		Stdout:    string(out),
		Stderr:    rerr.tail(),
		Duration:  time.Since(start),
		Truncated: truncated,
	}

	retErr := readErr
	if retErr == nil {
		retErr = splitExitError(waitErr, result)
	}

	e.logger.Debug("exec: completed", "cmd", name, "exit", result.ExitCode, "duration", result.Duration)
	return result, retErr
}

// RunWithStdinChecked is like RunChecked but pipes input to the command's stdin.
func (e *Executor) RunWithStdinChecked(ctx context.Context, input, name string, args ...string) (*Result, error) {
	result, err := e.RunWithStdin(ctx, input, name, args...)
	if err != nil {
		return result, err
	}
	if result.ExitCode != 0 {
		return result, NewExitError(ctx, name, result.ExitCode, result.Stderr)
	}
	return result, nil
}

// AppendEnv appends KEY=VALUE entries to the executor's env after
// construction; all post-construction env mutation must go through this
// method so future invariants (allowlist filtering, redaction) have one
// enforcement point.
func (e *Executor) AppendEnv(kvs ...string) {
	e.env = append(e.env, kvs...)
}

// SnapshotEnv returns a copy of the current env slice so callers can't
// mutate the executor's internal env through it. ZeroizeEnv only scrubs the
// live e.env — copies already handed out (e.g. terraform.WithEnv) need
// their own ZeroizeEnv to bound credential lifetime.
func (e *Executor) SnapshotEnv() []string {
	out := make([]string, len(e.env))
	copy(out, e.env)
	return out
}

// ZeroizeEnv blanks logutil.KeyIsSecret entries, then clears and nils the
// slice, bounding credential plaintext lifetime instead of leaving it on
// the heap until GC. Call via defer after subprocess operations complete.
func (e *Executor) ZeroizeEnv() {
	for i, kv := range e.env {
		key, _, _ := strings.Cut(kv, "=")
		if logutil.KeyIsSecret(key) {
			e.env[i] = ""
		}
	}
	clear(e.env)
	e.env = nil
}

// CommandExists reports whether name resolves on the current PATH.
func CommandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
