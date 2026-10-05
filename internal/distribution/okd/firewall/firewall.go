// Package firewall manages the firewalld rules OKD provisioning needs on the
// bastion host.
package firewall

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"slices"

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

// goos and isServiceActiveFn are test seams for DetectBackend's platform
// gate and firewalld probe.
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

// haproxyFrontends is the authoritative {number,protocol} list HAProxy binds
// on the bastion; explicit protocol prevents a same-number UDP rule slipping in.
var haproxyFrontends = []Port{
	{Number: phase.KubeAPIPort, Protocol: protoTCP, Description: "kubernetes api"},
	{Number: 22623, Protocol: protoTCP, Description: "machine config server"},
	{Number: 80, Protocol: protoTCP, Description: "http ingress"},
	{Number: 443, Protocol: protoTCP, Description: "https ingress"},
}

// HAProxyFrontendPorts returns the ports HAProxy binds on the bastion, as a
// defensive copy.
func HAProxyFrontendPorts() []Port {
	return slices.Clone(haproxyFrontends)
}

// Port describes a single firewall rule: number, protocol, and a logging
// description.
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

// DetectBackend returns Firewalld when firewall-cmd is present and the
// service is active, else None (always None off Linux).
func (f *Firewall) DetectBackend(ctx context.Context) Backend {
	if goos != "linux" {
		return None
	}
	if _, err := exec.LookPath("firewall-cmd"); err == nil && isServiceActiveFn(ctx, "firewalld") {
		return Firewalld
	}
	return None
}

// Configure opens each port in ports in firewalld; permanent persists the
// rules across reloads, and an inactive firewalld no-ops.
func (f *Firewall) Configure(ctx context.Context, ports []Port, permanent bool) error {
	if f.DetectBackend(ctx) == None {
		f.logger.Info("firewall: firewalld not active, skipping configuration")
		return nil
	}

	f.logger.Info("firewall: configuring", "backend", Firewalld)

	for _, port := range ports {
		if err := modifyPort(ctx, port, permanent, actionAdd); err != nil {
			return fmt.Errorf("open port %d: %w", port.Number, err)
		}
		f.logger.Info("firewall: opened port", "port", port.Number, "proto", port.Protocol, "desc", port.Description)
	}

	if permanent {
		if err := executor.RunCaptured(ctx, "firewall-cmd", "--reload"); err != nil {
			return fmt.Errorf("reload firewall: %w", err)
		}
	}

	f.logger.Info("firewall: configured")

	return nil
}

// validatePort allowlists Port.Protocol (tcp/udp) and the port range before
// modifyPort embeds it into a shell argument — a future caller must not skip this.
func validatePort(port Port) error {
	if port.Number < 1 || port.Number > 65535 {
		return fmt.Errorf("invalid port number: %d", port.Number)
	}
	if port.Protocol != protoTCP && port.Protocol != protoUDP {
		return fmt.Errorf("invalid protocol: %q (must be tcp or udp)", port.Protocol)
	}
	return nil
}

// RemoveRules deletes each port in ports from firewalld. Missing rules are
// logged as warnings rather than returned as errors.
func (f *Firewall) RemoveRules(ctx context.Context, ports []Port, permanent bool) error {
	if f.DetectBackend(ctx) == None {
		return nil
	}

	f.logger.Info("firewall: removing rules")

	for _, port := range ports {
		if err := modifyPort(ctx, port, permanent, actionRemove); err != nil {
			f.logger.Warn("firewall: could not remove port", "port", port.Number, "err", err)
		}
	}

	if permanent {
		// A failed reload leaves removed rules live in the runtime set; warn
		// but stay best-effort since teardown must not fail.
		if err := executor.RunCaptured(ctx, "firewall-cmd", "--reload"); err != nil {
			f.logger.Warn("firewall: reload after rule removal failed", "err", err)
		}
	}

	return nil
}

// modifyPort adds or removes a single firewalld rule. action is actionAdd or actionRemove.
func modifyPort(ctx context.Context, port Port, permanent bool, action string) error {
	if err := validatePort(port); err != nil {
		return err
	}

	flag := "--add-port="
	if action == actionRemove {
		flag = "--remove-port="
	}
	args := []string{flag + fmt.Sprintf("%d/%s", port.Number, port.Protocol)}
	if permanent {
		args = append(args, "--permanent")
	}
	// Port/protocol validated by validatePort above; args are an argv
	// slice (no shell interpolation).
	return executor.RunCaptured(ctx, "firewall-cmd", args...)
}

// ConfigureOKD opens all ports in OKDRequiredPorts.
func (f *Firewall) ConfigureOKD(ctx context.Context, permanent bool) error {
	return f.Configure(ctx, OKDRequiredPorts, permanent)
}

// RemoveOKDRules removes all ports in OKDRequiredPorts.
func (f *Firewall) RemoveOKDRules(ctx context.Context, permanent bool) error {
	return f.RemoveRules(ctx, OKDRequiredPorts, permanent)
}
