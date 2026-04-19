// Package executor wraps os/exec to run shell and sudo commands with
// context cancellation, timeouts, stream capture, and structured logging
// used across setup, install, and postinstall phases.
package executor

import (
	"bytes"
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
type Executor struct {
	WorkDir string
	Env     []string
	Stdout  io.Writer
	Stderr  io.Writer
	Verbose bool
	logger  *slog.Logger
}

// Option configures an Executor at construction time.
type Option func(*Executor)

// WithWorkDir sets the working directory for commands run by the Executor.
func WithWorkDir(dir string) Option {
	return func(e *Executor) { e.WorkDir = dir }
}

// WithEnv appends environment variables; they are merged with os.Environ()
// at execution time rather than replacing the inherited environment.
func WithEnv(env []string) Option {
	return func(e *Executor) { e.Env = append(e.Env, env...) }
}

// WithLogger injects a structured logger used for command-trace output.
// Nil logger falls back to logutil.NopLogger.
func WithLogger(l *slog.Logger) Option {
	return func(e *Executor) { e.logger = logutil.OrNop(l) }
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

	if len(e.Env) > 0 {
		cmd.Env = append(os.Environ(), e.Env...)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdin = stdin
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if e.Verbose {
		e.logger.Debug(fmt.Sprintf("+ %s %s", name, strings.Join(args, " ")))
	}

	err := cmd.Run()

	result := &Result{
		ExitCode: 0,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
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

// RunInteractive executes a command wired to the current process's stdin and
// the Executor's Stdout/Stderr for user-facing prompts.
func (e *Executor) RunInteractive(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)

	if e.WorkDir != "" {
		cmd.Dir = e.WorkDir
	}

	if len(e.Env) > 0 {
		cmd.Env = append(os.Environ(), e.Env...)
	}

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
