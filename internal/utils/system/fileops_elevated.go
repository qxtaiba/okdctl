package system

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// criticalPaths are system directories that should never be removed.
var criticalPaths = []string{"/", "/etc", "/var", "/usr", "/bin", "/sbin", "/lib", "/home", "/root", "/boot", "/dev", "/proc", "/sys"}

// isCriticalPath checks if a path is a critical system directory.
func isCriticalPath(path string) bool {
	cleanPath := filepath.Clean(path)
	for _, p := range criticalPaths {
		if cleanPath == p {
			return true
		}
	}
	return false
}

// FileOperation represents the type of file operation to perform.
type FileOperation int

const (
	OpCopy FileOperation = iota
	OpChmod
	OpChown
	OpMkdir
	OpRemove
)

// ExecuteFileOperation performs a file operation with automatic sudo fallback.
func ExecuteFileOperation(ctx context.Context, op FileOperation, target string, description string, args ...string) error {
	var regularOp, sudoOp func() error

	switch op {
	case OpCopy:
		if len(args) < 1 {
			return fmt.Errorf("copy operation requires source path")
		}
		source := args[0]
		regularOp = func() error {
			return CopyFile(source, target)
		}
		sudoOp = func() error {
			return runSudo("cp", source, target)
		}

	case OpChmod:
		if len(args) < 1 {
			return fmt.Errorf("chmod operation requires permissions")
		}
		perms := args[0]
		regularOp = func() error {
			return runCommand("chmod", perms, target)
		}
		sudoOp = func() error {
			return runSudo("chmod", perms, target)
		}

	case OpChown:
		if len(args) < 1 {
			return fmt.Errorf("chown operation requires owner")
		}
		owner := args[0]
		regularOp = func() error {
			return runCommand("chown", owner, target)
		}
		sudoOp = func() error {
			return runSudo("chown", owner, target)
		}

	case OpMkdir:
		regularOp = func() error {
			return os.MkdirAll(target, 0755)
		}
		sudoOp = func() error {
			return runSudo("mkdir", "-p", target)
		}

	case OpRemove:
		// Validate path is not a critical system directory
		if isCriticalPath(target) {
			return fmt.Errorf("refusing to remove critical system path: %s", target)
		}
		regularOp = func() error {
			return os.RemoveAll(target)
		}
		sudoOp = func() error {
			return runSudo("rm", "-rf", target)
		}

	default:
		return fmt.Errorf("unknown file operation: %d", op)
	}

	return ExecuteWithElevation(ctx, regularOp, sudoOp, description)
}

// CopyFileWithElevation copies a file with automatic sudo fallback.
func CopyFileWithElevation(ctx context.Context, src, dst, description string) error {
	return ExecuteFileOperation(ctx, OpCopy, dst, description, src)
}

// Chmod changes file permissions with automatic sudo fallback.
func Chmod(ctx context.Context, path, perms, description string) error {
	return ExecuteFileOperation(ctx, OpChmod, path, description, perms)
}

// Chown changes file ownership with automatic sudo fallback.
func Chown(ctx context.Context, path, owner, description string) error {
	return ExecuteFileOperation(ctx, OpChown, path, description, owner)
}

// MkdirAll creates a directory tree with automatic sudo fallback.
func MkdirAll(ctx context.Context, path, description string) error {
	return ExecuteFileOperation(ctx, OpMkdir, path, description)
}

// RemoveAll removes a file or directory with automatic sudo fallback.
func RemoveAll(ctx context.Context, path, description string) error {
	return ExecuteFileOperation(ctx, OpRemove, path, description)
}

// Helper functions

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	return cmd.Run()
}

// runSudo is the internal unexported version for use within the system package.
// It uses a background context by default for backwards compatibility with internal callers.
func runSudo(name string, args ...string) error {
	return RunSudo(context.Background(), name, args...)
}

// RunSudo runs a command with sudo privileges.
// Connects stdin/stdout/stderr to the terminal so sudo can prompt for password if needed.
// The context parameter allows for cancellation of long-running operations.
// Note: If already running as root, sudo simply runs the command directly (no password needed).
func RunSudo(ctx context.Context, name string, args ...string) error {
	sudoArgs := append([]string{name}, args...)
	cmd := exec.CommandContext(ctx, "sudo", sudoArgs...)
	// Connect to terminal so sudo can prompt for password if needed
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
