package system

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
)

// ServiceAction represents a systemd service action.
type ServiceAction string

const (
	ServiceEnable  ServiceAction = "enable"
	ServiceDisable ServiceAction = "disable"
	ServiceStart   ServiceAction = "start"
	ServiceStop    ServiceAction = "stop"
	ServiceRestart ServiceAction = "restart"
	ServiceStatus  ServiceAction = "status"
)

// ManageService controls a systemd service with the specified action.
func ManageService(ctx context.Context, action ServiceAction, serviceName, description string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("systemd services are only supported on Linux")
	}

	actionStr := string(action)

	switch action {
	case ServiceStatus:
		// Status check doesn't need sudo
		cmd := exec.CommandContext(ctx, "systemctl", "is-active", serviceName)
		return cmd.Run()

	default:
		// Other actions need sudo
		return RunSudo(ctx, "systemctl", actionStr, serviceName)
	}
}

// IsServiceActive checks if a systemd service is currently active.
func IsServiceActive(serviceName string) bool {
	if runtime.GOOS != "linux" {
		return false
	}

	cmd := exec.Command("systemctl", "is-active", "--quiet", serviceName)
	return cmd.Run() == nil
}

// IsServiceEnabled checks if a systemd service is enabled.
func IsServiceEnabled(serviceName string) bool {
	if runtime.GOOS != "linux" {
		return false
	}

	cmd := exec.Command("systemctl", "is-enabled", "--quiet", serviceName)
	return cmd.Run() == nil
}
