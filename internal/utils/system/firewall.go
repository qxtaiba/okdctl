package system

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

type FirewallBackend string

const (
	FirewallFirewalld FirewallBackend = "firewalld"
	FirewallUFW       FirewallBackend = "ufw"
	FirewallIPTables  FirewallBackend = "iptables"
	FirewallNone      FirewallBackend = "none"
)

var OKDRequiredPorts = []FirewallPort{
	{Port: 53, Protocol: "udp", Description: "dns"},
	{Port: 53, Protocol: "tcp", Description: "dns"},
	{Port: 6443, Protocol: "tcp", Description: "kubernetes api"},
	{Port: 22623, Protocol: "tcp", Description: "machine config server"},
	{Port: 80, Protocol: "tcp", Description: "http ingress"},
	{Port: 443, Protocol: "tcp", Description: "https ingress"},
	{Port: 8080, Protocol: "tcp", Description: "ignition server"},
}

type FirewallPort struct {
	Port        int
	Protocol    string // tcp, udp
	Description string
}

func DetectFirewallBackend() FirewallBackend {
	if runtime.GOOS != "linux" {
		return FirewallNone
	}

	// Check for firewalld (RHEL/Fedora/CentOS)
	if _, err := exec.LookPath("firewall-cmd"); err == nil {
		if IsServiceActive("firewalld") {
			return FirewallFirewalld
		}
	}

	// Check for ufw (Ubuntu/Debian)
	if _, err := exec.LookPath("ufw"); err == nil {
		cmd := exec.Command("ufw", "status")
		if output, err := cmd.Output(); err == nil {
			if strings.Contains(string(output), "Status: active") {
				return FirewallUFW
			}
		}
	}

	// Check for iptables
	if _, err := exec.LookPath("iptables"); err == nil {
		return FirewallIPTables
	}

	return FirewallNone
}

func ConfigureFirewall(ctx context.Context, ports []FirewallPort, permanent bool, logger utils.Logger) error {
	backend := DetectFirewallBackend()

	if backend == FirewallNone {
		logger.Warn("no active firewall detected, skipping firewall configuration")
		return nil
	}

	logger.Info(fmt.Sprintf("firewall: configuring using %s", backend))

	for _, port := range ports {
		if err := openPort(ctx, backend, port, permanent, logger); err != nil {
			return utils.WrapErrorf(err, "failed to open port %d", port.Port)
		}
	}

	if backend == FirewallFirewalld && permanent {
		if err := runSudo("firewall-cmd", "--reload"); err != nil {
			return utils.WrapError("failed to reload firewall", err)
		}
	}

	logger.Info("firewall: configured successfully")

	return nil
}

func validateFirewallPort(port FirewallPort) error {
	if port.Port < 1 || port.Port > 65535 {
		return fmt.Errorf("invalid port number: %d", port.Port)
	}
	if port.Protocol != "tcp" && port.Protocol != "udp" {
		return fmt.Errorf("invalid protocol: %s (must be tcp or udp)", port.Protocol)
	}
	return nil
}

func openPort(ctx context.Context, backend FirewallBackend, port FirewallPort, permanent bool, logger utils.Logger) error {
	if err := validateFirewallPort(port); err != nil {
		return err
	}

	portStr := fmt.Sprintf("%d/%s", port.Port, port.Protocol)

	switch backend {
	case FirewallFirewalld:
		args := []string{"firewall-cmd", fmt.Sprintf("--add-port=%s", portStr)}
		if permanent {
			args = append(args, "--permanent")
		}
		if err := runSudo(args[0], args[1:]...); err != nil {
			return err
		}

	case FirewallUFW:
		if err := runSudo("ufw", "allow", portStr); err != nil {
			return err
		}

	case FirewallIPTables:
		args := []string{
			"iptables", "-I", "INPUT", "-p", port.Protocol,
			"--dport", fmt.Sprintf("%d", port.Port), "-j", "ACCEPT",
		}
		if err := runSudo(args[0], args[1:]...); err != nil {
			return err
		}
	}

	logger.Info(fmt.Sprintf("firewall: opened port %d/%s (%s)", port.Port, port.Protocol, port.Description))

	return nil
}

func RemoveFirewallRules(ctx context.Context, ports []FirewallPort, permanent bool, logger utils.Logger) error {
	backend := DetectFirewallBackend()

	if backend == FirewallNone {
		return nil
	}

	logger.Info("firewall: removing rules")

	for _, port := range ports {
		if err := closePort(ctx, backend, port, permanent); err != nil {
			logger.Warn(fmt.Sprintf("could not remove port %d: %v", port.Port, err))
		}
	}

	if backend == FirewallFirewalld && permanent {
		_ = runSudo("firewall-cmd", "--reload")
	}

	return nil
}

func closePort(ctx context.Context, backend FirewallBackend, port FirewallPort, permanent bool) error {
	if err := validateFirewallPort(port); err != nil {
		return err
	}

	portStr := fmt.Sprintf("%d/%s", port.Port, port.Protocol)

	switch backend {
	case FirewallFirewalld:
		args := []string{"firewall-cmd", fmt.Sprintf("--remove-port=%s", portStr)}
		if permanent {
			args = append(args, "--permanent")
		}
		return runSudo(args[0], args[1:]...)

	case FirewallUFW:
		return runSudo("ufw", "delete", "allow", portStr)

	case FirewallIPTables:
		args := []string{
			"iptables", "-D", "INPUT", "-p", port.Protocol,
			"--dport", fmt.Sprintf("%d", port.Port), "-j", "ACCEPT",
		}
		return runSudo(args[0], args[1:]...)
	}

	return nil
}

func ConfigureOKDFirewall(ctx context.Context, permanent bool, logger utils.Logger) error {
	return ConfigureFirewall(ctx, OKDRequiredPorts, permanent, logger)
}

func RemoveOKDFirewallRules(ctx context.Context, permanent bool, logger utils.Logger) error {
	return RemoveFirewallRules(ctx, OKDRequiredPorts, permanent, logger)
}
