package system

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

var criticalPaths = []string{"/", "/etc", "/var", "/usr", "/bin", "/sbin", "/lib", "/home", "/root", "/boot", "/dev", "/proc", "/sys"}

func isCriticalPath(path string) bool {
	cleanPath := filepath.Clean(path)
	for _, p := range criticalPaths {
		if cleanPath == p {
			return true
		}
	}
	return false
}

type FileOperation int

const (
	OpCopy FileOperation = iota
	OpChmod
	OpChown
	OpMkdir
	OpRemove
)

func ExecuteFileOperation(ctx context.Context, op FileOperation, target, description string, args ...string) error {
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
			return runSudo(ctx, "cp", source, target)
		}

	case OpChmod:
		if len(args) < 1 {
			return fmt.Errorf("chmod operation requires permissions")
		}
		perms := args[0]
		regularOp = func() error {
			return runCommand(ctx, "chmod", perms, target)
		}
		sudoOp = func() error {
			return runSudo(ctx, "chmod", perms, target)
		}

	case OpChown:
		if len(args) < 1 {
			return fmt.Errorf("chown operation requires owner")
		}
		owner := args[0]
		regularOp = func() error {
			return runCommand(ctx, "chown", owner, target)
		}
		sudoOp = func() error {
			return runSudo(ctx, "chown", owner, target)
		}

	case OpMkdir:
		regularOp = func() error {
			return os.MkdirAll(target, 0o755)
		}
		sudoOp = func() error {
			return runSudo(ctx, "mkdir", "-p", target)
		}

	case OpRemove:
		if isCriticalPath(target) {
			return fmt.Errorf("refusing to remove critical system path: %s", target)
		}
		regularOp = func() error {
			return os.RemoveAll(target)
		}
		sudoOp = func() error {
			return runSudo(ctx, "rm", "-rf", target)
		}

	default:
		return fmt.Errorf("unknown file operation: %d", op)
	}

	return ExecuteWithElevation(ctx, regularOp, sudoOp, description)
}

func CopyFileWithElevation(ctx context.Context, src, dst, description string) error {
	return ExecuteFileOperation(ctx, OpCopy, dst, description, src)
}

func Chmod(ctx context.Context, path, perms, description string) error {
	return ExecuteFileOperation(ctx, OpChmod, path, description, perms)
}

func Chown(ctx context.Context, path, owner, description string) error {
	return ExecuteFileOperation(ctx, OpChown, path, description, owner)
}

func MkdirAll(ctx context.Context, path, description string) error {
	return ExecuteFileOperation(ctx, OpMkdir, path, description)
}

func RemoveAll(ctx context.Context, path, description string) error {
	return ExecuteFileOperation(ctx, OpRemove, path, description)
}

func runCommand(ctx context.Context, name string, args ...string) error {
	return exec.CommandContext(ctx, name, args...).Run()
}

func runSudo(ctx context.Context, name string, args ...string) error {
	return RunSudo(ctx, name, args...)
}

func RunSudo(ctx context.Context, name string, args ...string) error {
	sudoArgs := append([]string{name}, args...)
	cmd := exec.CommandContext(ctx, "sudo", sudoArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// HasPasswordlessSudo returns nil if `sudo -n true` succeeds, meaning the
// current user can run sudo without a password prompt (either via NOPASSWD
// or a cached timestamp). Otherwise it returns the underlying error. Callers
// typically log a warning on failure rather than blocking, because an earlier
// interactive run may have primed the sudo timestamp.
func HasPasswordlessSudo(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "sudo", "-n", "true")
	// Detach stdio so a password prompt can't hang here.
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}
