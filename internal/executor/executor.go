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
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

type Executor struct {
	WorkDir string
	Env     []string
	Stdout  io.Writer
	Stderr  io.Writer
	Verbose bool
	logger  utils.Logger
}

type Option func(*Executor)

func WithWorkDir(dir string) Option {
	return func(e *Executor) { e.WorkDir = dir }
}

func WithVerbose(verbose bool) Option {
	return func(e *Executor) { e.Verbose = verbose }
}

// WithEnv appends environment variables; they are merged with os.Environ()
// at execution time rather than replacing the inherited environment.
func WithEnv(env []string) Option {
	return func(e *Executor) { e.Env = append(e.Env, env...) }
}

// WithLogger injects a structured logger used for command-trace output.
// If the provided logger is nil the executor keeps its existing (noop) logger.
func WithLogger(l utils.Logger) Option {
	return func(e *Executor) {
		if l != nil {
			e.logger = l
		}
	}
}

func New(opts ...Option) *Executor {
	e := &Executor{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		logger: utils.NoopLogger(),
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

type Result struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
}

// Run executes a command and returns its result. The returned *Result is
// always non-nil, even when error is non-nil — callers can safely access
// result.ExitCode and result.Stderr without a nil guard.
func (e *Executor) Run(ctx context.Context, name string, args ...string) (*Result, error) {
	return e.run(ctx, nil, name, args...)
}

// RunWithStdin executes a command with the given string piped to its stdin.
// This is useful for commands like "oc create -f -" that read from stdin.
func (e *Executor) RunWithStdin(ctx context.Context, input string, name string, args ...string) (*Result, error) {
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
func (e *Executor) RunChecked(ctx context.Context, name string, args ...string) (*Result, error) {
	result, err := e.Run(ctx, name, args...)
	if err != nil {
		return result, err
	}
	if result.ExitCode != 0 {
		return result, fmt.Errorf("%s failed (exit %d): %s", name, result.ExitCode, strings.TrimSpace(result.Stderr))
	}
	return result, nil
}

// RunWithStdinChecked is like RunChecked but pipes input to the command's stdin.
func (e *Executor) RunWithStdinChecked(ctx context.Context, input string, name string, args ...string) (*Result, error) {
	result, err := e.RunWithStdin(ctx, input, name, args...)
	if err != nil {
		return result, err
	}
	if result.ExitCode != 0 {
		return result, fmt.Errorf("%s failed (exit %d): %s", name, result.ExitCode, strings.TrimSpace(result.Stderr))
	}
	return result, nil
}

func CommandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func CommandPath(name string) (string, error) {
	return exec.LookPath(name)
}
