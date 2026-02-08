// Package executor provides wrappers for executing external commands.
package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/qxtaiba/okd-proxmox-cli/internal/logging"
)

// Executor runs external commands with configurable working directory,
// environment variables, and output handling.
type Executor struct {
	// WorkDir is the working directory for commands.
	WorkDir string

	// Env contains additional environment variables.
	Env []string

	// Stdout is where stdout is written (default: os.Stdout).
	Stdout io.Writer

	// Stderr is where stderr is written (default: os.Stderr).
	Stderr io.Writer

	// Verbose enables verbose output.
	Verbose bool

	// logger is the logger for verbose output.
	logger logging.Logger
}

// Option configures an Executor.
type Option func(*Executor)

// WithWorkDir sets the working directory for commands.
func WithWorkDir(dir string) Option {
	return func(e *Executor) { e.WorkDir = dir }
}

// WithVerbose enables verbose output.
func WithVerbose(verbose bool) Option {
	return func(e *Executor) { e.Verbose = verbose }
}

// WithEnv adds environment variables to the executor.
// These are appended to the current environment when running commands.
func WithEnv(env []string) Option {
	return func(e *Executor) { e.Env = append(e.Env, env...) }
}

// New creates a new executor with optional configuration.
func New(opts ...Option) *Executor {
	e := &Executor{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		logger: logging.NoopLogger(),
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Result contains the outcome of a command execution, including exit code,
// captured output streams, and execution duration.
type Result struct {
	// ExitCode is the exit code of the command.
	ExitCode int

	// Stdout is the captured stdout.
	Stdout string

	// Stderr is the captured stderr.
	Stderr string

	// Duration is how long the command took.
	Duration time.Duration
}

// Run executes a command and returns the result.
func (e *Executor) Run(ctx context.Context, name string, args ...string) (*Result, error) {
	start := time.Now()

	cmd := exec.CommandContext(ctx, name, args...)

	if e.WorkDir != "" {
		cmd.Dir = e.WorkDir
	}

	if len(e.Env) > 0 {
		cmd.Env = append(os.Environ(), e.Env...)
	}

	var stdout, stderr bytes.Buffer
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

// RunInteractive runs a command with interactive I/O.
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

// RunWithOutput runs a command and returns stdout as a string.
func (e *Executor) RunWithOutput(ctx context.Context, name string, args ...string) (string, error) {
	result, err := e.Run(ctx, name, args...)
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return result.Stdout, fmt.Errorf("command failed with exit code %d: %s", result.ExitCode, result.Stderr)
	}
	return strings.TrimSpace(result.Stdout), nil
}

// CommandExists checks if a command exists in PATH.
func CommandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// CommandPath returns the full path to a command if it exists in PATH.
func CommandPath(name string) (string, error) {
	return exec.LookPath(name)
}
