// Package firewall manages host firewall rules required for OKD provisioning,
// abstracting over firewalld, ufw, and iptables backends.
package firewall

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

type Backend string

const (
	Firewalld Backend = "firewalld"
	UFW       Backend = "ufw"
	IPTables  Backend = "iptables"
	None      Backend = "none"
)

var OKDRequiredPorts = []Port{
	{Number: 53, Protocol: "udp", Description: "dns"},
	{Number: 53, Protocol: "tcp", Description: "dns"},
	{Number: 6443, Protocol: "tcp", Description: "kubernetes api"},
	{Number: 22623, Protocol: "tcp", Description: "machine config server"},
	{Number: 80, Protocol: "tcp", Description: "http ingress"},
	{Number: 443, Protocol: "tcp", Description: "https ingress"},
	{Number: 8080, Protocol: "tcp", Description: "ignition server"},
}

type Port struct {
	Number      int
	Protocol    string // tcp, udp
	Description string
}

func DetectBackend() Backend {
	if runtime.GOOS != "linux" {
		return None
	}

	if _, err := exec.LookPath("firewall-cmd"); err == nil {
		if system.IsServiceActive("firewalld") {
			return Firewalld
		}
	}

	if _, err := exec.LookPath("ufw"); err == nil {
		cmd := exec.Command("ufw", "status")
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

func Configure(ctx context.Context, ports []Port, permanent bool, logger utils.Logger) error {
	backend := DetectBackend()

	if backend == None {
		logger.Warn("no active firewall detected, skipping firewall configuration")
		return nil
	}

	logger.Info(fmt.Sprintf("firewall: configuring using %s", backend))

	for _, port := range ports {
		if err := openPort(ctx, backend, port, permanent, logger); err != nil {
			return utils.WrapErrorf(err, "failed to open port %d", port.Number)
		}
	}

	if backend == Firewalld && permanent {
		if err := system.RunSudo(ctx, "firewall-cmd", "--reload"); err != nil {
			return utils.WrapError("failed to reload firewall", err)
		}
	}

	logger.Info("firewall: configured successfully")

	return nil
}

func validatePort(port Port) error {
	if port.Number < 1 || port.Number > 65535 {
		return fmt.Errorf("invalid port number: %d", port.Number)
	}
	if port.Protocol != "tcp" && port.Protocol != "udp" {
		return fmt.Errorf("invalid protocol: %s (must be tcp or udp)", port.Protocol)
	}
	return nil
}

func openPort(ctx context.Context, backend Backend, port Port, permanent bool, logger utils.Logger) error {
	if err := validatePort(port); err != nil {
		return err
	}

	portStr := fmt.Sprintf("%d/%s", port.Number, port.Protocol)

	switch backend {
	case Firewalld:
		args := []string{"firewall-cmd", fmt.Sprintf("--add-port=%s", portStr)}
		if permanent {
			args = append(args, "--permanent")
		}
		if err := system.RunSudo(ctx, args[0], args[1:]...); err != nil {
			return err
		}

	case UFW:
		if err := system.RunSudo(ctx, "ufw", "allow", portStr); err != nil {
			return err
		}

	case IPTables:
		args := []string{
			"iptables", "-I", "INPUT", "-p", port.Protocol,
			"--dport", fmt.Sprintf("%d", port.Number), "-j", "ACCEPT",
		}
		if err := system.RunSudo(ctx, args[0], args[1:]...); err != nil {
			return err
		}
	}

	logger.Info(fmt.Sprintf("firewall: opened port %d/%s (%s)", port.Number, port.Protocol, port.Description))

	return nil
}

func RemoveRules(ctx context.Context, ports []Port, permanent bool, logger utils.Logger) error {
	backend := DetectBackend()

	if backend == None {
		return nil
	}

	logger.Info("firewall: removing rules")

	for _, port := range ports {
		if err := closePort(ctx, backend, port, permanent); err != nil {
			logger.Warn(fmt.Sprintf("could not remove port %d: %v", port.Number, err))
		}
	}

	if backend == Firewalld && permanent {
		_ = system.RunSudo(ctx, "firewall-cmd", "--reload")
	}

	return nil
}

func closePort(ctx context.Context, backend Backend, port Port, permanent bool) error {
	if err := validatePort(port); err != nil {
		return err
	}

	portStr := fmt.Sprintf("%d/%s", port.Number, port.Protocol)

	switch backend {
	case Firewalld:
		args := []string{"firewall-cmd", fmt.Sprintf("--remove-port=%s", portStr)}
		if permanent {
			args = append(args, "--permanent")
		}
		return system.RunSudo(ctx, args[0], args[1:]...)

	case UFW:
		return system.RunSudo(ctx, "ufw", "delete", "allow", portStr)

	case IPTables:
		args := []string{
			"iptables", "-D", "INPUT", "-p", port.Protocol,
			"--dport", fmt.Sprintf("%d", port.Number), "-j", "ACCEPT",
		}
		return system.RunSudo(ctx, args[0], args[1:]...)
	}

	return nil
}

func ConfigureOKD(ctx context.Context, permanent bool, logger utils.Logger) error {
	return Configure(ctx, OKDRequiredPorts, permanent, logger)
}

func RemoveOKDRules(ctx context.Context, permanent bool, logger utils.Logger) error {
	return RemoveRules(ctx, OKDRequiredPorts, permanent, logger)
}
