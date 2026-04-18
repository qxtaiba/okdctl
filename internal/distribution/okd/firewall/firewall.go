// Package firewall manages host firewall rules required for OKD provisioning,
// abstracting over firewalld, ufw, and iptables backends.
package firewall

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"strings"

	"github.com/qxtaiba/okdctl/internal/system"
)

type Backend string

const (
	Firewalld Backend = "firewalld"
	UFW       Backend = "ufw"
	IPTables  Backend = "iptables"
	None      Backend = "none"
)

const actionRemove = "remove"

// OKDRequiredPorts is the authoritative port list opened by setup;
// HAProxyFrontendPorts() derives its subset from this slice.
var OKDRequiredPorts = []Port{
	{Number: 53, Protocol: "udp", Description: "dns"},
	{Number: 53, Protocol: "tcp", Description: "dns"},
	{Number: 6443, Protocol: "tcp", Description: "kubernetes api"},
	{Number: 22623, Protocol: "tcp", Description: "machine config server"},
	{Number: 80, Protocol: "tcp", Description: "http ingress"},
	{Number: 443, Protocol: "tcp", Description: "https ingress"},
	{Number: 8080, Protocol: "tcp", Description: "ignition server"},
}

// haproxyPortNumbers is the set of port numbers HAProxy binds on the bastion.
var haproxyPortNumbers = map[int]bool{6443: true, 22623: true, 80: true, 443: true}

// HAProxyFrontendPorts returns the subset of OKDRequiredPorts that HAProxy
// binds on the bastion. Postinstall uses this to tear down firewall rules when
// HAProxy is removed, without touching DNS and ignition rules.
func HAProxyFrontendPorts() []Port {
	var ports []Port
	for _, p := range OKDRequiredPorts {
		if haproxyPortNumbers[p.Number] && p.Protocol == "tcp" {
			ports = append(ports, p)
		}
	}
	return ports
}

type Port struct {
	Number      int
	Protocol    string // tcp, udp
	Description string
}

func DetectBackend(ctx context.Context) Backend {
	if runtime.GOOS != "linux" {
		return None
	}

	if _, err := exec.LookPath("firewall-cmd"); err == nil {
		if system.IsServiceActive(ctx, "firewalld") {
			return Firewalld
		}
	}

	if _, err := exec.LookPath("ufw"); err == nil {
		cmd := exec.CommandContext(ctx, "ufw", "status")
		if output, err := cmd.Output(); err == nil {
			if strings.Contains(string(output), "Status: active") {
				return UFW
			}
		}
	}

	if _, err := exec.LookPath("iptables"); err == nil {
		return IPTables
	}

	return None
}

func Configure(ctx context.Context, ports []Port, permanent bool, logger *slog.Logger) error {
	backend := DetectBackend(ctx)

	if backend == None {
		logger.Warn("no active firewall detected, skipping firewall configuration")
		return nil
	}

	logger.Info(fmt.Sprintf("firewall: configuring using %s", backend))

	for _, port := range ports {
		if err := openPort(ctx, backend, port, permanent, logger); err != nil {
			return fmt.Errorf("failed to open port %d: %w", port.Number, err)
		}
	}

	if backend == Firewalld && permanent {
		if err := exec.CommandContext(ctx, "firewall-cmd", "--reload").Run(); err != nil {
			return fmt.Errorf("failed to reload firewall: %w", err)
		}
	}

	logger.Info("firewall: configured successfully")

	return nil
}

// validatePort enforces an allowlist on Port.Protocol (tcp or udp only) and
// a valid port number range. Both openPort and closePort MUST call this
// before embedding port.Protocol into a firewall-cmd / iptables / ufw
// argument, because the protocol value flows into fmt.Sprintf("%d/%s", ...).
// Even though current callers only ever populate Port from the
// OKDRequiredPorts / HAProxyFrontendPorts constants, keeping the guard here
// prevents a future caller from sneaking an unvalidated protocol string into
// the rendered rule.
func validatePort(port Port) error {
	if port.Number < 1 || port.Number > 65535 {
		return fmt.Errorf("invalid port number: %d", port.Number)
	}
	if port.Protocol != "tcp" && port.Protocol != "udp" {
		return fmt.Errorf("invalid protocol: %q (must be tcp or udp)", port.Protocol)
	}
	return nil
}

func openPort(ctx context.Context, backend Backend, port Port, permanent bool, logger *slog.Logger) error {
	if err := modifyPort(ctx, backend, port, permanent, "add"); err != nil {
		return err
	}
	logger.Info(fmt.Sprintf("firewall: opened port %d/%s (%s)", port.Number, port.Protocol, port.Description))
	return nil
}

func RemoveRules(ctx context.Context, ports []Port, permanent bool, logger *slog.Logger) error {
	backend := DetectBackend(ctx)

	if backend == None {
		return nil
	}

	logger.Info("firewall: removing rules")

	for _, port := range ports {
		if err := modifyPort(ctx, backend, port, permanent, actionRemove); err != nil {
			logger.Warn(fmt.Sprintf("could not remove port %d: %v", port.Number, err))
		}
	}

	if backend == Firewalld && permanent {
		_ = exec.CommandContext(ctx, "firewall-cmd", "--reload").Run()
	}

	return nil
}

// modifyPort adds or removes a single firewall rule. action is "add" or "remove".
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
		return exec.CommandContext(ctx, args[0], args[1:]...).Run()

	case UFW:
		if action == actionRemove {
			return exec.CommandContext(ctx, "ufw", "delete", "allow", portStr).Run()
		}
		return exec.CommandContext(ctx, "ufw", "allow", portStr).Run()

	case IPTables:
		chainAction := "-I"
		if action == actionRemove {
			chainAction = "-D"
		}
		args := []string{
			"iptables", chainAction, "INPUT", "-p", port.Protocol,
			"--dport", fmt.Sprintf("%d", port.Number), "-j", "ACCEPT",
		}
		return exec.CommandContext(ctx, args[0], args[1:]...).Run()
	}

	return nil
}

func ConfigureOKD(ctx context.Context, permanent bool, logger *slog.Logger) error {
	return Configure(ctx, OKDRequiredPorts, permanent, logger)
}

func RemoveOKDRules(ctx context.Context, permanent bool, logger *slog.Logger) error {
	return RemoveRules(ctx, OKDRequiredPorts, permanent, logger)
}
