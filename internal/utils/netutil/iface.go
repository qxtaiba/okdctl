// Package netutil provides IP math (CIDR arithmetic, host enumeration,
// VIP derivation) and host interface operations used when provisioning
// cluster networking.
package netutil

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

func RemoveSecondaryIP(ctx context.Context, ip, iface string) error {
	if ip == "" {
		return fmt.Errorf("ip address is required")
	}
	if iface == "" {
		return fmt.Errorf("interface name is required")
	}

	checkCmd := exec.CommandContext(ctx, "ip", "addr", "show", "dev", iface)
	output, err := checkCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to check IP presence on device %s: %w", iface, err)
	}
	if !strings.Contains(string(output), ip) {
		return nil
	}

	conn, err := connectionForDevice(ctx, iface)
	if err != nil {
		return fmt.Errorf("failed to find networkmanager connection for %s: %w", iface, err)
	}

	if err := system.RunSudo(ctx, "nmcli", "connection", "modify", conn, "-ipv4.addresses", ip+"/32"); err != nil {
		return fmt.Errorf("failed to remove IP %s from connection %s: %w", ip, conn, err)
	}

	if err := system.RunSudo(ctx, "nmcli", "device", "reapply", iface); err != nil {
		return fmt.Errorf("failed to apply IP change on %s: %w", iface, err)
	}

	return nil
}

func GetDefaultInterface(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "ip", "route", "show", "default")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get default route: %w", err)
	}

	fields := strings.Fields(string(output))
	for i, field := range fields {
		if field == "dev" && i+1 < len(fields) {
			return fields[i+1], nil
		}
	}

	return "", fmt.Errorf("could not determine default interface from route: %s", string(output))
}

func connectionForDevice(ctx context.Context, iface string) (string, error) {
	cmd := exec.CommandContext(ctx, "nmcli", "-t", "-f", "NAME,DEVICE", "connection", "show", "--active")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to list networkmanager connections: %w", err)
	}

	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 && parts[1] == iface {
			return parts[0], nil
		}
	}

	return "", fmt.Errorf("no active networkmanager connection found for device %s", iface)
}
