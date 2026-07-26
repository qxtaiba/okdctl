// Package hostnet mutates host network state by shelling out to ip and
// nmcli, which typically requires root. Pure IP/CIDR arithmetic lives in
// internal/netutil; keep anything that touches the host out of that package.
package hostnet

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/qxtaiba/okdctl/internal/executor"
)

// validConnectionNameRegex is the allowlist for nmcli connection names:
// the realistic NetworkManager name space (alphanumerics, space, dot,
// underscore, slash, colon for aliases like br0:1, hyphen, up to 128
// chars), rejecting everything else including all shell metacharacters.
// The explicit leading-dash refusal closes CWE-88: nmcli treats a
// leading-dash token as a property selector in argv position.
var validConnectionNameRegex = regexp.MustCompile(`^[A-Za-z0-9 ._/:-]{1,128}$`)

func validateConnectionName(name string) error {
	if name == "" {
		return fmt.Errorf("connection name must not be empty")
	}
	if name[0] == '-' {
		return fmt.Errorf("connection name %q must not start with a dash", name)
	}
	if !validConnectionNameRegex.MatchString(name) {
		return fmt.Errorf("connection name %q does not match allowed character set", name)
	}
	return nil
}

// RemoveSecondaryIP strips ip from the active NetworkManager connection bound
// to iface and reapplies the device. No-ops when ip is not currently assigned.
func RemoveSecondaryIP(ctx context.Context, ip, iface string) error {
	if ip == "" {
		return fmt.Errorf("ip address is required")
	}
	if iface == "" {
		return fmt.Errorf("interface name is required")
	}

	output, err := executor.OutputCaptured(ctx, "ip", "addr", "show", "dev", iface)
	if err != nil {
		return fmt.Errorf("check IP presence on device %s: %w", iface, err)
	}
	if !strings.Contains(string(output), ip+"/") {
		return nil
	}

	conn, err := connectionForDevice(ctx, iface)
	if err != nil {
		return fmt.Errorf("find networkmanager connection for %s: %w", iface, err)
	}

	if err := executor.RunCaptured(ctx, "nmcli", "connection", "modify", conn, "-ipv4.addresses", ip+"/32"); err != nil {
		return fmt.Errorf("remove IP %s from connection %s: %w", ip, conn, err)
	}

	if err := executor.RunCaptured(ctx, "nmcli", "device", "reapply", iface); err != nil {
		// The profile was already modified; without a compensating re-add the
		// persistent config and the running device silently diverge until the
		// next reconnect. Restore the profile, and if even that fails name the
		// divergence so the operator knows the profile no longer matches the
		// runtime state.
		if restoreErr := executor.RunCaptured(ctx, "nmcli", "connection", "modify", conn, "+ipv4.addresses", ip+"/32"); restoreErr != nil {
			return fmt.Errorf("apply IP change on %s: %w (profile %s no longer lists %s but the device was not reapplied; restoring the profile also failed: %w)", iface, err, conn, ip, restoreErr)
		}
		return fmt.Errorf("apply IP change on %s: %w (profile change on %s was rolled back)", iface, err, conn)
	}

	return nil
}

// ActiveConnection returns the name of the first active non-loopback
// NetworkManager connection. The name is scraped from nmcli output, so it
// is validated against the connection-name allowlist before it can reach
// any nmcli argv position.
func ActiveConnection(ctx context.Context) (string, error) {
	out, err := executor.OutputCaptured(ctx, "nmcli", "-t", "-f", "NAME", "connection", "show", "--active")
	if err != nil {
		return "", fmt.Errorf("list network connections: %w", err)
	}

	for line := range strings.Lines(string(out)) {
		line = strings.TrimSpace(line)
		if line == "" || line == "lo" {
			continue
		}
		if err := validateConnectionName(line); err != nil {
			return "", fmt.Errorf("active connection name rejected: %w", err)
		}
		return line, nil
	}
	return "", fmt.Errorf("no active network connection found")
}

// OverrideConnectionDNS points conn's IPv4 DNS at servers and disables
// automatic (DHCP) DNS on the profile. The change is persisted only; call
// ActivateConnection to apply it to the running device.
func OverrideConnectionDNS(ctx context.Context, conn string, servers []string) error {
	if err := validateConnectionName(conn); err != nil {
		return fmt.Errorf("connection name rejected: %w", err)
	}
	return executor.RunCaptured(ctx, "nmcli", "connection", "modify", conn,
		"ipv4.dns", strings.Join(servers, ","), "ipv4.ignore-auto-dns", "yes")
}

// ClearConnectionDNSOverride reverts OverrideConnectionDNS: it empties the
// profile's IPv4 DNS list and re-enables automatic DNS.
func ClearConnectionDNSOverride(ctx context.Context, conn string) error {
	if err := validateConnectionName(conn); err != nil {
		return fmt.Errorf("connection name rejected: %w", err)
	}
	return executor.RunCaptured(ctx, "nmcli", "connection", "modify", conn,
		"ipv4.dns", "", "ipv4.ignore-auto-dns", "no")
}

// ActivateConnection brings conn up so profile changes take effect on the
// running device.
func ActivateConnection(ctx context.Context, conn string) error {
	if err := validateConnectionName(conn); err != nil {
		return fmt.Errorf("connection name rejected: %w", err)
	}
	return executor.RunCaptured(ctx, "nmcli", "connection", "up", conn)
}

// GetDefaultInterface returns the interface name that carries the host's
// default IPv4 route.
func GetDefaultInterface(ctx context.Context) (string, error) {
	output, err := executor.OutputCaptured(ctx, "ip", "route", "show", "default")
	if err != nil {
		return "", fmt.Errorf("get default route: %w", err)
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
	output, err := executor.OutputCaptured(ctx, "nmcli", "-t", "-f", "NAME,DEVICE", "connection", "show", "--active")
	if err != nil {
		return "", fmt.Errorf("list networkmanager connections: %w", err)
	}

	for line := range strings.Lines(string(output)) {
		line = strings.TrimRight(line, "\n")
		unescaped := strings.ReplaceAll(line, `\:`, "\x00")
		parts := strings.SplitN(unescaped, ":", 2)
		if len(parts) == 2 && parts[1] == iface {
			conn := strings.ReplaceAll(parts[0], "\x00", ":")
			if err := validateConnectionName(conn); err != nil {
				return "", fmt.Errorf("networkmanager connection name rejected: %w", err)
			}
			return conn, nil
		}
	}

	return "", fmt.Errorf("no active networkmanager connection found for device %s", iface)
}
