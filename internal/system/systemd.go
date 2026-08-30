package system

import (
	"context"
	"errors"
	"runtime"

	"github.com/qxtaiba/okdctl/internal/executor"
)

const osLinux = "linux"

// ServiceAction names a systemctl verb, passed to systemctl verbatim. One
// exception: ManageService rewrites ServiceStatus to `is-active`.
type ServiceAction string

// ServiceActions passable to ManageService.
const (
	ServiceEnable  ServiceAction = "enable"
	ServiceDisable ServiceAction = "disable"
	ServiceStart   ServiceAction = "start"
	ServiceStop    ServiceAction = "stop"
	ServiceRestart ServiceAction = "restart"
	ServiceReload  ServiceAction = "reload"
	ServiceStatus  ServiceAction = "status"
)

// ManageService invokes systemctl for the given service on Linux, erroring
// on other platforms rather than silently no-op-ing. ServiceStatus is
// rewritten to `is-active`, an exit-code probe for programmatic gating,
// instead of the human-readable `status` report which pages and returns
// non-zero for healthy-but-inactive units.
func ManageService(ctx context.Context, action ServiceAction, serviceName string) error {
	if runtime.GOOS != osLinux {
		return errors.New("systemd services are only supported on Linux")
	}

	if action == ServiceStatus {
		return executor.RunCaptured(ctx, "systemctl", "is-active", serviceName)
	}
	return executor.RunCaptured(ctx, "systemctl", string(action), serviceName)
}

// IsServiceActive reports whether systemctl considers the service running.
// Returns false on non-Linux hosts rather than erroring — callers use it as
// a gate, not a diagnostic.
func IsServiceActive(ctx context.Context, serviceName string) bool {
	if runtime.GOOS != osLinux {
		return false
	}

	return executor.RunCaptured(ctx, "systemctl", "is-active", "--quiet", serviceName) == nil
}

// IsServiceEnabled reports whether systemctl considers the service enabled
// for boot-time start. Returns false on non-Linux hosts (see IsServiceActive).
func IsServiceEnabled(ctx context.Context, serviceName string) bool {
	if runtime.GOOS != osLinux {
		return false
	}

	return executor.RunCaptured(ctx, "systemctl", "is-enabled", "--quiet", serviceName) == nil
}
