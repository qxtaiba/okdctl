package system

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

// SudoAvailable checks if passwordless sudo is available.
func SudoAvailable() bool {
	if runtime.GOOS == "windows" {
		return false
	}

	cmd := exec.Command("sudo", "-n", "true")
	return cmd.Run() == nil
}

// isElevationNeeded checks if an error indicates we need elevated privileges.
// This checks for os.ErrPermission, exec.ExitError, and specific permission-related error messages.
func isElevationNeeded(err error) bool {
	if errors.Is(err, os.ErrPermission) {
		return true
	}

	// Check for exec.ExitError (shell commands like chmod return this on permission errors)
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return true
	}

	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "permission denied") ||
		strings.Contains(errStr, "operation not permitted")
}

// ExecuteWithElevation attempts an operation without sudo first,
// then falls back to sudo if the operation fails with a permission error.
// Note: sudo may prompt for a password interactively if passwordless sudo is not configured.
func ExecuteWithElevation(ctx context.Context, operation func() error, sudoOperation func() error, description string) error {
	err := operation()
	if err == nil {
		return nil
	}

	if !isElevationNeeded(err) {
		return err
	}

	if err := sudoOperation(); err != nil {
		return utils.WrapErrorf(err, "%s failed even with sudo", description)
	}

	return nil
}
