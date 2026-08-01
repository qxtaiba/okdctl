package system

import (
	"context"
	"fmt"
	"runtime"

	"github.com/qxtaiba/okdctl/internal/executor"
)

const osLinux = "linux"

// ServiceAction names a systemctl verb. Values are passed to systemctl
// verbatim so they must stay lowercase and syntactically valid, with one
// exception: ManageService maps ServiceStatus to `is-active` (see there).
type ServiceAction string

// ServiceActions passable to ManageService. Each maps to a systemctl
// subcommand verbatim except ServiceStatus, which ManageService rewrites to
// `is-active`.
const (
	ServiceEnable  ServiceAction = "enable"
	ServiceDisable ServiceAction = "disable"
	ServiceStart   ServiceAction = "start"
	ServiceStop    ServiceAction = "stop"
	ServiceRestart ServiceAction = "restart"
	ServiceReload  ServiceAction = "reload"
	ServiceStatus  ServiceAction = "status"
)

// ManageService invokes systemctl for the given service on Linux. Non-Linux
// hosts get an error rather than a silent no-op so callers don't assume the
// action took effect. ServiceStatus is rewritten to `is-active` — an
// exit-code probe suitable for programmatic gating — rather than the
// human-readable `status` report, which pages and returns non-zero for
// inactive-but-healthy units.
func ManageService(ctx context.Context, action ServiceAction, serviceName string) error {
	if runtime.GOOS != osLinux {
		return fmt.Errorf("systemd services are only supported on Linux")
	}

	actionStr := string(action)

	switch action {
	case ServiceStatus:
		return executor.RunCaptured(ctx, "systemctl", "is-active", serviceName)

	default:
		return executor.RunCaptured(ctx, "systemctl", actionStr, serviceName)
	}
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
