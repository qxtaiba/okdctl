// Package firewall manages host firewall rules required for OKD provisioning,
// abstracting over firewalld, ufw, and iptables backends.
package firewall

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/system"
)

// Backend identifies which host firewall implementation is active.
type Backend string

// Backend values recognised by DetectBackend.
const (
	Firewalld Backend = "firewalld"
	UFW       Backend = "ufw"
	IPTables  Backend = "iptables"
	None      Backend = "none"
)

const (
	actionAdd    = "add"
	actionRemove = "remove"
)

const (
	protoTCP = "tcp"
	protoUDP = "udp"
)

// goos and isServiceActiveFn are test seams: DetectBackend's platform gate
// and firewalld-active probe are otherwise unreachable from non-Linux hosts.
var (
	goos              = runtime.GOOS
	isServiceActiveFn = system.IsServiceActive
)

// OKDRequiredPorts is the authoritative port list opened by setup;
// HAProxyFrontendPorts() derives its subset from this slice.
var OKDRequiredPorts = []Port{
	{Number: 53, Protocol: protoUDP, Description: "dns"},
	{Number: 53, Protocol: protoTCP, Description: "dns"},
	{Number: phase.KubeAPIPort, Protocol: protoTCP, Description: "kubernetes api"},
	{Number: 22623, Protocol: protoTCP, Description: "machine config server"},
	{Number: 80, Protocol: protoTCP, Description: "http ingress"},
	{Number: 443, Protocol: protoTCP, Description: "https ingress + ignition server"},
}

// haproxyFrontends is the authoritative list of {number, protocol} pairs
// HAProxy binds on the bastion. Protocol is explicit so a future UDP rule
// on the same number cannot be silently included.
var haproxyFrontends = []Port{
	{Number: phase.KubeAPIPort, Protocol: protoTCP, Description: "kubernetes api"},
	{Number: 22623, Protocol: protoTCP, Description: "machine config server"},
	{Number: 80, Protocol: protoTCP, Description: "http ingress"},
	{Number: 443, Protocol: protoTCP, Description: "https ingress"},
}

// HAProxyFrontendPorts returns the ports HAProxy binds on the bastion.
// Postinstall uses this to tear down firewall rules when HAProxy is removed,
// without touching DNS and ignition rules. The result is a defensive copy.
func HAProxyFrontendPorts() []Port {
	return slices.Clone(haproxyFrontends)
}

// Port describes a single firewall rule: number + protocol, with a
// human-readable description used for logging.
type Port struct {
	Number      int
	Protocol    string // tcp, udp
	Description string
}

// Firewall applies and removes host firewall rules using the active backend.
type Firewall struct {
	logger *slog.Logger
}

// Option configures a Firewall at construction time.
type Option func(*Firewall)

// WithLogger injects a structured logger. Nil resolves to logutil.NopLogger.
func WithLogger(l *slog.Logger) Option {
	return func(f *Firewall) { f.logger = logutil.OrNop(l) }
}

// New builds a Firewall with a no-op logger, then applies opts.
func New(opts ...Option) *Firewall {
	f := &Firewall{logger: logutil.NopLogger}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// DetectBackend returns the active firewall backend, preferring firewalld,
// then ufw, then iptables. Returns None on non-Linux hosts or when no
// backend is present.
func (f *Firewall) DetectBackend(ctx context.Context) Backend {
	if goos != "linux" {
		return None
	}

	if _, err := exec.LookPath("firewall-cmd"); err == nil {
		if isServiceActiveFn(ctx, "firewalld") {
			return Firewalld
		}
	}

	if _, err := exec.LookPath("ufw"); err == nil {
		if output, err := executor.OutputCaptured(ctx, "ufw", "status"); err == nil {
			if strings.Contains(string(output), "Status: active") {
				return UFW
			}
		} else {
			f.logger.Debug("ufw probe failed, falling through to next backend", "err", err, "backend", "ufw")
		}
	}

	if _, err := exec.LookPath("iptables"); err == nil {
		return IPTables
	}

	return None
}

// Configure opens each port in ports on the active backend. When permanent
// is true, firewalld rules persist across reloads. A None backend no-ops.
func (f *Firewall) Configure(ctx context.Context, ports []Port, permanent bool) error {
	backend := f.DetectBackend(ctx)

	if backend == None {
		f.logger.Info("firewall: no active backend detected, skipping configuration")
		return nil
	}

	f.logger.Info("firewall: configuring", "backend", backend)

	for _, port := range ports {
		if err := openPort(ctx, backend, port, permanent, f.logger); err != nil {
			return fmt.Errorf("open port %d: %w", port.Number, err)
		}
	}

	if backend == Firewalld && permanent {
		if err := executor.RunCaptured(ctx, "firewall-cmd", "--reload"); err != nil {
			return fmt.Errorf("reload firewall: %w", err)
		}
	}

	f.logger.Info("firewall: configured")

	return nil
}

// validatePort enforces an allowlist on Port.Protocol (tcp or udp only) and
// a valid port number range. modifyPort MUST call this before embedding
// port.Protocol into a firewall-cmd / iptables / ufw argument, since the
// value flows into fmt.Sprintf("%d/%s", ...). Current callers only populate
// Port from OKDRequiredPorts / HAProxyFrontendPorts, but the guard stays so
// a future caller cannot slip an unvalidated protocol string into the rule.
func validatePort(port Port) error {
	if port.Number < 1 || port.Number > 65535 {
		return fmt.Errorf("invalid port number: %d", port.Number)
	}
	if port.Protocol != protoTCP && port.Protocol != protoUDP {
		return fmt.Errorf("invalid protocol: %q (must be tcp or udp)", port.Protocol)
	}
	return nil
}

func openPort(ctx context.Context, backend Backend, port Port, permanent bool, logger *slog.Logger) error {
	logger = logutil.OrNop(logger)
	if err := modifyPort(ctx, backend, port, permanent, actionAdd); err != nil {
		return err
	}
	logger.Info("firewall: opened port", "port", port.Number, "proto", port.Protocol, "desc", port.Description)
	return nil
}

// RemoveRules deletes each port in ports from the active backend. Missing
// rules are logged as warnings rather than returned as errors.
func (f *Firewall) RemoveRules(ctx context.Context, ports []Port, permanent bool) error {
	backend := f.DetectBackend(ctx)

	if backend == None {
		return nil
	}

	f.logger.Info("firewall: removing rules")

	for _, port := range ports {
		if err := modifyPort(ctx, backend, port, permanent, actionRemove); err != nil {
			f.logger.Warn("firewall: could not remove port", "port", port.Number, "err", err)
		}
	}

	if backend == Firewalld && permanent {
		// A failed reload leaves the just-removed permanent rules live in the
		// runtime set; surface it so the operator does not read "removing
		// rules" as "ports closed". Stay best-effort — teardown must not fail.
		if err := executor.RunCaptured(ctx, "firewall-cmd", "--reload"); err != nil {
			f.logger.Warn("firewall: reload after rule removal failed", "err", err)
		}
	}

	return nil
}

// modifyPort adds or removes a single firewall rule. action is actionAdd or actionRemove.
func modifyPort(ctx context.Context, backend Backend, port Port, permanent bool, action string) error {
	if err := validatePort(port); err != nil {
		return err
	}

	portStr := fmt.Sprintf("%d/%s", port.Number, port.Protocol)

	switch backend {
	case Firewalld:
		flag := "--add-port="
		if action == actionRemove {
			flag = "--remove-port="
		}
		args := []string{"firewall-cmd", flag + portStr}
		if permanent {
			args = append(args, "--permanent")
		}
		// Port/protocol validated by validatePort above; args are an argv
		// slice (no shell interpolation).
		return executor.RunCaptured(ctx, args[0], args[1:]...)

	case UFW:
		if action == actionRemove {
			return executor.RunCaptured(ctx, "ufw", "delete", "allow", portStr)
		}
		return executor.RunCaptured(ctx, "ufw", "allow", portStr)

	case IPTables:
		return modifyIptablesRule(ctx, port, action)
	}

	return nil
}

// modifyIptablesRule makes the iptables backend idempotent. Plain -I inserts a
// duplicate on every re-run and -D removes only one instance, so StepConfigure
// ReRunSafeYes would otherwise leave N-1 ACCEPT rules after N setups that a
// single destroy pass cannot clear. A -C probe gates the add; a -D loop drains
// every duplicate on remove. Port/protocol already validated by validatePort;
// argv slice (no shell).
func modifyIptablesRule(ctx context.Context, port Port, action string) error {
	rule := []string{"INPUT", "-p", port.Protocol, "--dport", strconv.Itoa(port.Number), "-j", "ACCEPT"}
	exists := func() bool {
		return executor.RunCaptured(ctx, "iptables", append([]string{"-C"}, rule...)...) == nil
	}
	if action == actionRemove {
		for exists() {
			if err := executor.RunCaptured(ctx, "iptables", append([]string{"-D"}, rule...)...); err != nil {
				return err
			}
		}
		return nil
	}
	if exists() {
		return nil
	}
	return executor.RunCaptured(ctx, "iptables", append([]string{"-I"}, rule...)...)
}

// ConfigureOKD opens all ports in OKDRequiredPorts.
func (f *Firewall) ConfigureOKD(ctx context.Context, permanent bool) error {
	return f.Configure(ctx, OKDRequiredPorts, permanent)
}

// RemoveOKDRules removes all ports in OKDRequiredPorts.
func (f *Firewall) RemoveOKDRules(ctx context.Context, permanent bool) error {
	return f.RemoveRules(ctx, OKDRequiredPorts, permanent)
}
