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

func SudoAvailable() bool {
	if runtime.GOOS == "windows" {
		return false
	}

	cmd := exec.Command("sudo", "-n", "true")
	return cmd.Run() == nil
}

func isElevationNeeded(err error) bool {
	if errors.Is(err, os.ErrPermission) {
		return true
	}

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
