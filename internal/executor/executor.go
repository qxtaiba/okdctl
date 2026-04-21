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
	"strings"
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
// RunStreamedChecked, which tee live output to e.Stdout/e.Stderr while
// still returning a ring-buffered tail in the Result.
//
// Environment handling: by default the Executor passes only a curated
// allowlist of the parent environment down to subprocesses — credentials
// that happen to be exported (unrelated AWS/GCP tokens, shell history
// plumbing, etc.) do not reach every shellout. Callers append extra
// vars via WithEnv. The rare caller that needs the full parent env
// (e.g. a tool that consumes a non-allowlisted variable) opts out via
// WithInheritedEnv.
type Executor struct {
	WorkDir    string
	Env        []string
	Stdout     io.Writer
	Stderr     io.Writer
	Verbose    bool
	inheritEnv bool
	logger     *slog.Logger
}

// Option configures an Executor at construction time.
type Option func(*Executor)

// WithWorkDir sets the working directory for commands run by the Executor.
func WithWorkDir(dir string) Option {
	return func(e *Executor) { e.WorkDir = dir }
}

// WithEnv appends environment variables; they are appended after the
// allowlist-filtered parent env (or the full inherited env when
// WithInheritedEnv is set), so caller-supplied keys win on duplicates.
func WithEnv(env []string) Option {
	return func(e *Executor) { e.Env = append(e.Env, env...) }
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
func WithInheritedEnv() Option {
	return func(e *Executor) { e.inheritEnv = true }
}

// New builds an Executor with defaults wired to os.Stdout/os.Stderr and a
// no-op logger, then applies the provided options.
func New(opts ...Option) *Executor {
	e := &Executor{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		logger: logutil.NopLogger,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// defaultEnvAllowlist controls which parent-env variables reach subprocesses
// when the caller has not opted into WithInheritedEnv. Errs generous:
// deploy needs tooling plumbing (HOME, PATH, KUBECONFIG), proxy vars,
// SUDO_* for the re-exec model, and every provider-prefixed namespace
// commonly used with terraform / openshift-install / oc / helm.
//
// Tighten this list only after a targeted audit finds a specific var
// leaking credentials in practice; every entry here gets a docstring
// justification if removed later.
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
	},
	prefixes: []string{
		"KUBE",       // KUBECONFIG, KUBE_*
		"OC_",        // openshift-client
		"TF_",        // terraform TF_VAR_*, TF_LOG, TF_PLUGIN_*
		"TERRAFORM_", // terraform built-ins
		"PROXMOX_",   // bpg/proxmox provider + PROXMOX_VE_*
		"HELM_",      // helm
		"GIT_",       // git auth + paths
		"GITHUB_",    // gh CLI
		"GH_",        // gh CLI
	},
}

// envAllowlist is a dual exact-match + prefix-match filter.
type envAllowlist struct {
	exact    map[string]bool
	prefixes []string
}

func (a envAllowlist) allows(key string) bool {
	if a.exact[key] {
		return true
	}
	for _, p := range a.prefixes {
		if strings.HasPrefix(key, p) {
			return true
		}
	}
	return false
}

// filterParentEnv returns the entries of os.Environ() whose keys pass the
// allowlist. Called once per Run; cheap at ~50-200 entries typical.
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

// buildEnv composes the subprocess env for a single Run. Inherit mode passes
// os.Environ() through unchanged. Default mode filters through
// defaultEnvAllowlist, then appends caller-supplied e.Env (later wins in
// the duplicate-key tie).
func (e *Executor) buildEnv() []string {
	if e.inheritEnv {
		if len(e.Env) == 0 {
			return nil // nil means os.Environ() by os/exec contract
		}
		return append(os.Environ(), e.Env...)
	}
	base := filterParentEnv(defaultEnvAllowlist)
	if len(e.Env) == 0 {
		return base
	}
	return append(base, e.Env...)
}

// Result is the captured outcome of a Run-style invocation.
type Result struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
}

// ExitError is the typed error RunChecked / RunWithStdinChecked return when
// a subprocess exits non-zero. Callers can errors.As to inspect ExitCode
// without re-parsing the error message, and errors.Is to compare against
// Unwrap chain values. Mirrors terraform.ExecError for consistency.
type ExitError struct {
	Command  string
	ExitCode int
	Stderr   string
}

// Error returns a human-readable form matching the legacy bare
// fmt.Errorf output so existing log/stringification sites are
// unchanged.
func (e *ExitError) Error() string {
	return fmt.Sprintf("%s failed (exit %d): %s", e.Command, e.ExitCode, strings.TrimSpace(e.Stderr))
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

	if e.WorkDir != "" {
		cmd.Dir = e.WorkDir
	}

	cmd.Env = e.buildEnv()

	rout := newRingWriter(constMaxLines)
	rerr := newRingWriter(constMaxLines)
	cmd.Stdin = stdin
	cmd.Stdout = rout
	cmd.Stderr = rerr

	if e.Verbose {
		e.logger.Debug(fmt.Sprintf("+ %s %s", name, strings.Join(args, " ")))
	}

	err := cmd.Run()

	result := &Result{
		ExitCode: 0,
		Stdout:   rout.tail(),
		Stderr:   rerr.tail(),
		Duration: time.Since(start),
	}

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		} else {
			return result, err
		}
	}

	return result, nil
}

// RunStreamed executes a command, piping its stdout and stderr live to
// e.Stdout and e.Stderr while also retaining the last constMaxLines lines
// in Result.Stdout and Result.Stderr for error reporting. The returned
// *Result is always non-nil.
func (e *Executor) RunStreamed(ctx context.Context, name string, args ...string) (*Result, error) {
	start := time.Now()
	cmd := exec.CommandContext(ctx, name, args...)

	if e.WorkDir != "" {
		cmd.Dir = e.WorkDir
	}
	cmd.Env = e.buildEnv()

	rout := newRingWriter(constMaxLines)
	rerr := newRingWriter(constMaxLines)
	cmd.Stdout = io.MultiWriter(e.Stdout, rout)
	cmd.Stderr = io.MultiWriter(e.Stderr, rerr)

	e.logger.Debug("exec: started", "cmd", name, "argc", len(args))
	err := cmd.Run()

	result := &Result{
		ExitCode: 0,
		Stdout:   rout.tail(),
		Stderr:   rerr.tail(),
		Duration: time.Since(start),
	}
	e.logger.Debug("exec: completed", "cmd", name, "duration", result.Duration)

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		} else {
			return result, err
		}
	}

	return result, nil
}

// RunStreamedChecked is RunStreamed with RunChecked semantics: non-zero
// exit returns an *ExitError carrying the tail of stderr.
func (e *Executor) RunStreamedChecked(ctx context.Context, name string, args ...string) (*Result, error) {
	result, err := e.RunStreamed(ctx, name, args...)
	if err != nil {
		return result, err
	}
	if result.ExitCode != 0 {
		return result, &ExitError{Command: name, ExitCode: result.ExitCode, Stderr: result.Stderr}
	}
	return result, nil
}

// RunInteractive executes a command wired to the current process's stdin and
// the Executor's Stdout/Stderr for user-facing prompts.
func (e *Executor) RunInteractive(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)

	if e.WorkDir != "" {
		cmd.Dir = e.WorkDir
	}

	cmd.Env = e.buildEnv()

	cmd.Stdin = os.Stdin
	cmd.Stdout = e.Stdout
	cmd.Stderr = e.Stderr

	if e.Verbose {
		e.logger.Debug(fmt.Sprintf("+ %s %s", name, strings.Join(args, " ")))
	}

	return cmd.Run()
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
		return result, &ExitError{Command: name, ExitCode: result.ExitCode, Stderr: result.Stderr}
	}
	return result, nil
}

// RunWithStdinChecked is like RunChecked but pipes input to the command's stdin.
func (e *Executor) RunWithStdinChecked(ctx context.Context, input, name string, args ...string) (*Result, error) {
	result, err := e.RunWithStdin(ctx, input, name, args...)
	if err != nil {
		return result, err
	}
	if result.ExitCode != 0 {
		return result, &ExitError{Command: name, ExitCode: result.ExitCode, Stderr: result.Stderr}
	}
	return result, nil
}

// CommandExists reports whether name resolves on the current PATH.
func CommandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
