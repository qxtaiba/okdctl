// Package executor provides wrappers for executing external commands.
package executor

import (
	"context"
)

// CommandExecutor defines the interface for running system commands.
// This interface enables testing by allowing mock implementations to be injected.
type CommandExecutor interface {
	// Run executes a command and returns the result.
	Run(ctx context.Context, cmd string, args ...string) (*Result, error)

	// RunWithOutput runs a command and returns stdout as a string.
	RunWithOutput(ctx context.Context, cmd string, args ...string) (string, error)
}

// Ensure Executor implements CommandExecutor.
var _ CommandExecutor = (*Executor)(nil)
