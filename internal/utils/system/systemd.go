package system

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
)

const osLinux = "linux"

type ServiceAction string

const (
	ServiceEnable  ServiceAction = "enable"
	ServiceDisable ServiceAction = "disable"
	ServiceStart   ServiceAction = "start"
	ServiceStop    ServiceAction = "stop"
	ServiceRestart ServiceAction = "restart"
	ServiceReload  ServiceAction = "reload"
	ServiceStatus  ServiceAction = "status"
)

func ManageService(ctx context.Context, action ServiceAction, serviceName, _ string) error {
	if runtime.GOOS != osLinux {
		return fmt.Errorf("systemd services are only supported on Linux")
	}

	actionStr := string(action)

	switch action {
	case ServiceStatus:
		cmd := exec.CommandContext(ctx, "systemctl", "is-active", serviceName)
		return cmd.Run()

	default:
		return RunSudo(ctx, "systemctl", actionStr, serviceName)
	}
}

func IsServiceActive(ctx context.Context, serviceName string) bool {
	if runtime.GOOS != osLinux {
		return false
	}

	cmd := exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", serviceName)
	return cmd.Run() == nil
}

func IsServiceEnabled(ctx context.Context, serviceName string) bool {
	if runtime.GOOS != osLinux {
		return false
	}

	cmd := exec.CommandContext(ctx, "systemctl", "is-enabled", "--quiet", serviceName)
	return cmd.Run() == nil
}
