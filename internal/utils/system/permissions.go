package system

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func SudoAvailable() bool {
	if runtime.GOOS == "windows" {
		return false
	}

	cmd := exec.CommandContext(context.Background(), "sudo", "-n", "true")
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

func ExecuteWithElevation(_ context.Context, operation, sudoOperation func() error, description string) error {
	err := operation()
	if err == nil {
		return nil
	}

	if !isElevationNeeded(err) {
		return err
	}

	if err := sudoOperation(); err != nil {
		return fmt.Errorf("%s failed even with sudo: %w", description, err)
	}

	return nil
}
