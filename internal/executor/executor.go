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
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
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

// ValidateExecResult validates an executor result and returns a formatted error if the command failed.
// This is a helper to reduce boilerplate error checking across the codebase.
func ValidateExecResult(cmdDescription string, result *Result, err error) error {
	if err != nil {
		return utils.WrapErrorf(err, "%s: execution failed", cmdDescription)
	}
	if result == nil {
		return fmt.Errorf("%s: nil result", cmdDescription)
	}
	if result.ExitCode != 0 {
		stderr := strings.TrimSpace(result.Stderr)
		if stderr == "" {
			stderr = strings.TrimSpace(result.Stdout)
		}
		if stderr == "" {
			return fmt.Errorf("%s: exited with code %d", cmdDescription, result.ExitCode)
		}
		return fmt.Errorf("%s: %s", cmdDescription, stderr)
	}
	return nil
}

// ═══════════════════════════════════════════════════════════════════════════════
// SUDO HELPERS
// ═══════════════════════════════════════════════════════════════════════════════

// SudoCopy copies a file using sudo.
func (e *Executor) SudoCopy(ctx context.Context, src, dst string) error {
	result, err := e.Run(ctx, "sudo", "cp", src, dst)
	return ValidateExecResult(fmt.Sprintf("copy %s to %s", src, dst), result, err)
}

// SudoSystemctl runs systemctl commands using sudo.
func (e *Executor) SudoSystemctl(ctx context.Context, action, service string) error {
	result, err := e.Run(ctx, "sudo", "systemctl", action, service)
	return ValidateExecResult(fmt.Sprintf("systemctl %s %s", action, service), result, err)
}

// RunSudoInteractive runs a command with sudo, connecting terminal I/O for password prompts.
// This is useful for commands that may require interactive sudo authentication.
func (e *Executor) RunSudoInteractive(ctx context.Context, name string, args ...string) error {
	sudoArgs := append([]string{name}, args...)
	cmd := exec.CommandContext(ctx, "sudo", sudoArgs...)

	if e.WorkDir != "" {
		cmd.Dir = e.WorkDir
	}

	if len(e.Env) > 0 {
		cmd.Env = append(os.Environ(), e.Env...)
	}

	// Connect to terminal so sudo can prompt for password if needed
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if e.Verbose {
		e.logger.Debug(fmt.Sprintf("+ sudo %s %s", name, strings.Join(args, " ")))
	}

	return cmd.Run()
}
