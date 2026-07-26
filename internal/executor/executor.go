// Package executor wraps os/exec to run shell and sudo commands with
// context cancellation, timeouts, stream capture, and structured logging
// used across setup, install, and postinstall phases.
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

// Executor wraps os/exec with context-aware command execution, output
// capture, and structured logging for setup/install/postinstall phases.
//
// Output capture: Run and RunChecked capture stdout/stderr into a ring
// buffer capped at constMaxLines (200) lines. Result.Stdout and
// Result.Stderr carry the tail of the output, not the full stream.
// Callers that need the full stream should use RunStreamed or
// RunStreamedChecked, which tee live output to e.stdout/e.stderr while
// still returning a ring-buffered tail in the Result. RunOutput/
// RunOutputChecked instead fully buffer stdout up to a byte cap for
// machine-parsed payloads; RunDiscard/RunDiscardChecked discard stdout
// entirely for high-volume/uninteresting output.
//
// Environment handling: by default the Executor passes only a curated
// allowlist of the parent environment down to subprocesses — credentials
// that happen to be exported (unrelated AWS/GCP tokens, shell history
// plumbing, etc.) do not reach every shellout. Parent entries whose key
// matches logutil.KeyIsSecret (PROXMOX_VE_PASSWORD, TF_VAR_*token, …) are
// dropped even when their namespace is allowlisted; a credential reaches a
// subprocess only when the caller passes it explicitly via WithEnv or
// AppendEnv. The rare caller that needs the full parent env (e.g. a tool
// that consumes a non-allowlisted variable) opts out via WithInheritedEnv.
//
// Must be constructed via New — the zero value panics on first use
// (logger and output streams are set only in New).
//
// Cancel signal: ctx cancellation sends cancelSignal to the subprocess
// (SIGTERM by default; see WithCancelSignal) followed by SIGKILL after a
// 30s WaitDelay if the process has not exited.
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

// WithEnv appends environment variables; they are appended after the
// allowlist-filtered parent env (or the full inherited env when
// WithInheritedEnv is set), so caller-supplied keys win on duplicates.
func WithEnv(env []string) Option {
	return func(e *Executor) { e.env = append(e.env, env...) }
}

// WithLogger injects a structured logger used for command-trace output.
// Nil logger falls back to logutil.NopLogger.
func WithLogger(l *slog.Logger) Option {
	return func(e *Executor) { e.logger = logutil.OrNop(l) }
}

// WithInheritedEnv disables the default env allowlist and passes the
// parent's full environment to subprocesses. Use sparingly — prefer
// WithEnv for well-known variables. Use cases: a tool that consumes a
// variable not on the allowlist, or a test that needs a custom env that
// the allowlist would filter.
//
// Takes no argument because every current call site wants unconditional
// inheritance; add a bool parameter (matching download.WithOverwrite's
// shape) only when a caller needs WithInheritedEnv(false) dynamic dispatch.
func WithInheritedEnv() Option {
	return func(e *Executor) { e.inheritEnv = true }
}

// WithCancelSignal overrides the signal cmd.Cancel sends on ctx
// cancellation. Defaults to SIGTERM (see New): SIGTERM soft-cancel is the
// default so DefaultEnvAllowlist-guarded subprocesses (oc, ssh, package
// managers) get a graceful chance to flush before WaitDelay's 30s SIGKILL
// escalation. SIGINT is terraform's documented soft-cancel: it triggers a
// graceful plan/apply abort and releases the state lock before exit — pass
// WithCancelSignal(syscall.SIGINT) only for a terraform-backed Executor.
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
// the sudo re-exec in internal/cli/elevation.go. It passes tooling plumbing
// and provider namespaces; everything else is dropped to prevent unrelated
// tokens reaching privileged processes.
//
// Executor subprocesses additionally drop secret-keyed entries from the
// filtered base (see buildEnv); only re-exec sites that call
// FilterParentEnv directly — okdctl handing the environment to itself —
// receive allowlisted credentials such as PROXMOX_VE_PASSWORD.
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
		// Broader GIT_/GITHUB_/GH_ prefixes are intentionally excluded —
		// GITHUB_TOKEN, GH_TOKEN, and GIT_ASKPASS carry credentials no
		// subprocess in this tree needs.
		"GIT_SSH_COMMAND": true, "GIT_TERMINAL_PROMPT": true,
	},
	Prefixes: []string{
		"KUBE",       // KUBECONFIG, KUBE_*
		"OC_",        // openshift-client
		"TF_",        // terraform TF_VAR_*, TF_LOG, TF_PLUGIN_*
		"TERRAFORM_", // terraform built-ins
		"PROXMOX_",   // bpg/proxmox provider + PROXMOX_VE_*
		"HELM_",      // helm
	},
}

// EnvAllowlist is a dual exact-match + prefix-match filter for environment
// variables. Exported so callers outside this package (e.g. cli/elevation.go)
// can reuse the same list rather than duplicating it.
type EnvAllowlist struct {
	Exact    map[string]bool
	Prefixes []string
}

func (a EnvAllowlist) allows(key string) bool {
	return a.Exact[key] || slices.ContainsFunc(a.Prefixes, func(p string) bool { return strings.HasPrefix(key, p) })
}

// FilterParentEnv returns the entries of os.Environ() whose keys pass the
// allowlist. Exported so the sudo re-exec path in cli/elevation.go can use
// the same filter without duplicating the allowlist.
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

// buildEnv composes the subprocess env for a single Run. Inherit mode passes
// os.Environ() through unchanged. Default mode filters through
// DefaultEnvAllowlist, drops secret-keyed entries, then appends
// caller-supplied e.env (later wins in the duplicate-key tie).
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

// dropSecretKeyed removes entries whose key matches logutil.KeyIsSecret.
// The PROXMOX_/TF_ namespaces are allowlisted for provider plumbing, not
// for broadcasting PROXMOX_VE_PASSWORD / PROXMOX_VE_API_TOKEN (which
// credentials.LoadEnvFile setenvs into the process) to every oc/ssh/
// package-manager shellout; executors that need credentials receive them
// explicitly via WithEnv(creds.Env()).
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

// Result is the captured outcome of a Run-style invocation.
// Truncated is true when stdout was capped: the ring paths (Run/RunChecked/
// RunStreamed) set it once a line is actually dropped past constMaxLines;
// the byte-cap path (RunOutput) sets it when output exceeds the limit.
// Callers that machine-parse stdout must use RunOutput/RunOutputChecked and
// check Truncated before unmarshalling.
type Result struct {
	ExitCode  int
	Stdout    string
	Stderr    string
	Duration  time.Duration
	Truncated bool
}

// ExitError is the typed error RunChecked / RunWithStdinChecked return when
// a subprocess exits non-zero. Callers can errors.As to inspect ExitCode
// without re-parsing the error message. Mirrors terraform.ExecError for
// consistency.
type ExitError struct {
	Command  string
	ExitCode int
	Stderr   string
}

// Error truncates Stderr to at most 400 bytes via logutil.RedactableStderr
// so a credential-bearing terraform provider diagnostic does not reach log
// sinks verbatim when a caller stringifies outside slog.
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

// NewExitError returns the ctx error when ctx is already cancelled so
// errors.Is(err, context.Canceled) propagates through the call chain,
// letting cli/root.go::signalExitCode map SIGINT→130 / SIGTERM→143
// instead of falling through to the generic ClusterError exit code 4.
func NewExitError(ctx context.Context, cmd string, code int, stderr string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return &ExitError{Command: cmd, ExitCode: code, Stderr: stderr}
}

// Run executes a command and returns its result. The returned *Result is
// always non-nil, even when error is non-nil — callers can safely access
// result.ExitCode and result.Stderr without a nil guard.
func (e *Executor) Run(ctx context.Context, name string, args ...string) (*Result, error) {
	return e.run(ctx, nil, name, args...)
}

// RunWithStdin executes a command with the given string piped to its stdin.
func (e *Executor) RunWithStdin(ctx context.Context, input, name string, args ...string) (*Result, error) {
	return e.run(ctx, strings.NewReader(input), name, args...)
}

func (e *Executor) run(ctx context.Context, stdin io.Reader, name string, args ...string) (*Result, error) {
	start := time.Now()

	cmd := exec.CommandContext(ctx, name, args...)

	if e.workDir != "" {
		cmd.Dir = e.workDir
	}

	cmd.Env = e.buildEnv()

	rout := newRingWriter(constMaxLines)
	rerr := newRingWriter(constMaxLines)
	cmd.Stdin = stdin
	cmd.Stdout = rout
	cmd.Stderr = rerr

	// Soft-cancel via e.cancelSignal (SIGTERM by default; see
	// WithCancelSignal) so the subprocess gets a chance to clean up before
	// WaitDelay's SIGKILL escalation.
	cmd.Cancel = func() error { return cmd.Process.Signal(e.cancelSignal) }
	cmd.WaitDelay = 30 * time.Second

	e.logger.Debug("exec: started", "cmd", name, "argc", len(args))

	err := cmd.Run()

	result := &Result{
		ExitCode:  0,
		Stdout:    rout.tail(),
		Stderr:    rerr.tail(),
		Duration:  time.Since(start),
		Truncated: rout.dropped,
	}

	var retErr error
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		} else {
			retErr = err
		}
	}

	e.logger.Debug("exec: completed", "cmd", name, "exit", result.ExitCode, "duration", result.Duration)
	return result, retErr
}

// RunStreamed executes a command, piping its stdout and stderr live to
// e.stdout and e.stderr while also retaining the last constMaxLines lines
// in Result.Stdout and Result.Stderr for error reporting. The returned
// *Result is always non-nil.
func (e *Executor) RunStreamed(ctx context.Context, name string, args ...string) (*Result, error) {
	start := time.Now()
	cmd := exec.CommandContext(ctx, name, args...)

	if e.workDir != "" {
		cmd.Dir = e.workDir
	}
	cmd.Env = e.buildEnv()

	rout := newRingWriter(constMaxLines)
	rerr := newRingWriter(constMaxLines)
	cmd.Stdout = io.MultiWriter(e.stdout, rout)
	cmd.Stderr = io.MultiWriter(e.stderr, rerr)

	cmd.Cancel = func() error { return cmd.Process.Signal(e.cancelSignal) }
	cmd.WaitDelay = 30 * time.Second

	e.logger.Debug("exec: started", "cmd", name, "argc", len(args))
	err := cmd.Run()

	result := &Result{
		ExitCode:  0,
		Stdout:    rout.tail(),
		Stderr:    rerr.tail(),
		Duration:  time.Since(start),
		Truncated: rout.dropped,
	}

	var retErr error
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		} else {
			retErr = err
		}
	}

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

// StartStreamed starts name with args, piping stdout/stderr live to
// e.stdout/e.stderr, and returns immediately with a channel that receives
// cmd.Wait's result once the process exits. Shares buildEnv, cancelSignal,
// and WaitDelay with the other Run* methods, so callers get the SIGTERM/
// SIGINT + WaitDelay escalation without reimplementing it. kill is a no-op
// retained for API symmetry with callers that inject a test stub expecting
// an explicit kill function; cmd.Cancel already handles ctx cancellation.
func (e *Executor) StartStreamed(ctx context.Context, name string, args ...string) (done <-chan error, kill func(), err error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if e.workDir != "" {
		cmd.Dir = e.workDir
	}
	cmd.Env = e.buildEnv()
	cmd.Stdout = e.stdout
	cmd.Stderr = e.stderr

	cmd.Cancel = func() error { return cmd.Process.Signal(e.cancelSignal) }
	cmd.WaitDelay = 30 * time.Second

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
	cmd := exec.CommandContext(ctx, name, args...)

	if e.workDir != "" {
		cmd.Dir = e.workDir
	}

	cmd.Env = e.buildEnv()

	cmd.Stdin = os.Stdin
	cmd.Stdout = e.stdout
	cmd.Stderr = e.stderr

	// Soft-cancel via e.cancelSignal (SIGTERM by default; see
	// WithCancelSignal). SIGINT is terraform's documented soft-cancel —
	// opted into via WithCancelSignal for terraform's Executor. WaitDelay
	// gives the process 30 s to clean up before SIGKILL fires.
	cmd.Cancel = func() error { return cmd.Process.Signal(e.cancelSignal) }
	cmd.WaitDelay = 30 * time.Second

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

// exitCodeOf extracts the exit code from a cmd.Run error for logging.
// Returns 0 for nil, the exit status for *exec.ExitError, and -1 for other
// errors (e.g. exec not found).
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

// RunChecked executes a command and returns an error if it fails to execute
// or exits with a non-zero status. Use this when the caller expects success;
// use Run directly when non-zero exit codes are acceptable (probing, cleanup).
// Non-zero exits return an *ExitError — callers can errors.As to inspect.
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

// RunDiscard executes a command with stdout fully discarded — not even
// ring-buffered — while stderr stays ring-capped at constMaxLines lines.
// Use for high-volume/uninteresting stdout (package installs, repo
// metadata refreshes) where only success/failure and stderr diagnostics
// matter. The returned *Result is always non-nil; Result.Stdout is always
// empty.
func (e *Executor) RunDiscard(ctx context.Context, name string, args ...string) (*Result, error) {
	start := time.Now()
	cmd := exec.CommandContext(ctx, name, args...)

	if e.workDir != "" {
		cmd.Dir = e.workDir
	}
	cmd.Env = e.buildEnv()

	rerr := newRingWriter(constMaxLines)
	cmd.Stdout = io.Discard
	cmd.Stderr = rerr

	cmd.Cancel = func() error { return cmd.Process.Signal(e.cancelSignal) }
	cmd.WaitDelay = 30 * time.Second

	e.logger.Debug("exec: started", "cmd", name, "argc", len(args))
	err := cmd.Run()

	result := &Result{
		Stderr:   rerr.tail(),
		Duration: time.Since(start),
	}

	var retErr error
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		} else {
			retErr = err
		}
	}

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

// RunOutput executes a command and returns the full stdout up to limit
// bytes. When limit is 0, runOutputMaxBytes (4 MiB) is used. Unlike Run,
// stdout is fully buffered rather than ring-truncated, making it safe for
// machine parsing of large JSON payloads (e.g. oc get clusteroperators -o
// json). Stderr stays ring-buffered for diagnostics. When stdout exceeds
// limit, Result.Truncated is true and Stdout holds the capped prefix. The
// returned *Result is always non-nil.
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

	cmd := exec.CommandContext(ctx, name, args...)
	if e.workDir != "" {
		cmd.Dir = e.workDir
	}
	cmd.Env = e.buildEnv()

	rerr := newRingWriter(constMaxLines)
	cmd.Stdin = stdin
	cmd.Stderr = rerr

	stdoutPipe, pipeErr := cmd.StdoutPipe()
	if pipeErr != nil {
		return &Result{Duration: time.Since(start)}, pipeErr
	}

	// Soft-cancel via e.cancelSignal mirrors run(); see WithCancelSignal for
	// the SIGTERM-default / terraform-SIGINT rationale.
	cmd.Cancel = func() error { return cmd.Process.Signal(e.cancelSignal) }
	cmd.WaitDelay = 30 * time.Second

	e.logger.Debug("exec: started", "cmd", name, "argc", len(args))

	if err := cmd.Start(); err != nil {
		return &Result{Duration: time.Since(start)}, err
	}

	// Read limit+1 bytes so "exactly limit" and "exceeded limit" are
	// distinguishable, then drain the rest: a child producing more than the
	// kernel pipe buffer past the cap would otherwise block forever in write
	// and Wait would never return.
	out, readErr := io.ReadAll(io.LimitReader(stdoutPipe, int64(limit)+1))
	_, _ = io.Copy(io.Discard, stdoutPipe)
	waitErr := cmd.Wait()

	truncated := len(out) > limit
	if truncated {
		out = out[:limit]
	}

	result := &Result{
		ExitCode:  0,
		Stdout:    string(out),
		Stderr:    rerr.tail(),
		Duration:  time.Since(start),
		Truncated: truncated,
	}

	var retErr error
	switch {
	case readErr != nil:
		retErr = readErr
	case waitErr != nil:
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		} else {
			retErr = waitErr
		}
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

// AppendEnv appends KEY=VALUE entries to the executor's env after construction.
// All post-construction env mutation must go through this method so future
// invariants (allowlist filtering, credential redaction on insert) have a
// single enforcement point.
func (e *Executor) AppendEnv(kvs ...string) {
	e.env = append(e.env, kvs...)
}

// SnapshotEnv returns a copy of the current env slice. The copy prevents
// callers from mutating the executor's internal env through the returned
// slice. Note: ZeroizeEnv operates on the live e.env slice; copies already
// handed to callers (e.g. terraform.WithEnv) are not reached by it — callers
// must call their own ZeroizeEnv to bound credential lifetime there.
func (e *Executor) SnapshotEnv() []string {
	out := make([]string, len(e.env))
	copy(out, e.env)
	return out
}

// ZeroizeEnv blanks env entries whose key matches logutil.KeyIsSecret, then
// clears and nils the slice. Credential-bearing strings (PROXMOX_VE_PASSWORD,
// PROXMOX_VE_API_TOKEN, etc.) would otherwise persist as immutable heap
// objects until GC; this bounds their plaintext lifetime. Call via defer
// after all subprocess operations are complete.
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
