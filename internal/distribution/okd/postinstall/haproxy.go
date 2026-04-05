package postinstall

import (
	"context"
	"fmt"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/firewall"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/netutil"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

// RemoveHAProxy stops and disables HAProxy on the bastion, removing it as the API load balancer.
// If vip is non-empty, the secondary IP is removed from the bastion's interface and the API
// is re-verified via the VIP after teardown to ensure kube-vip is handling traffic.
func (p *Phase) RemoveHAProxy(ctx context.Context, vip string) error {
	p.Log.Info("haproxy: stopping service")
	result, err := p.Exec.Run(ctx, "sudo", "systemctl", "stop", "haproxy")
	if err != nil {
		return utils.WrapError("failed to stop haproxy", err)
	}
	if result.ExitCode != 0 {
		p.Log.Warn(fmt.Sprintf("haproxy: stop returned non-zero exit code: %s", result.Stderr))
	}

	p.Log.Info("haproxy: disabling service")
	result, err = p.Exec.Run(ctx, "sudo", "systemctl", "disable", "haproxy")
	if err != nil {
		return utils.WrapError("failed to disable haproxy", err)
	}
	if result.ExitCode != 0 {
		p.Log.Warn(fmt.Sprintf("haproxy: disable returned non-zero exit code: %s", result.Stderr))
	}

	p.Log.Info("haproxy: removing configuration")
	_, err = p.Exec.Run(ctx, "sudo", "rm", "-f", "/etc/haproxy/haproxy.cfg")
	if err != nil {
		p.Log.Warn(fmt.Sprintf("haproxy: failed to remove config: %v", err))
	}

	for _, port := range firewall.HAProxyFrontendPorts {
		portSpec := fmt.Sprintf("%d/%s", port.Number, port.Protocol)
		p.Log.Info(fmt.Sprintf("haproxy: removing firewall rule for port %d", port.Number))
		result, err = p.Exec.Run(ctx, "sudo", "firewall-cmd", "--permanent", "--remove-port="+portSpec)
		if err != nil || result.ExitCode != 0 {
			p.Log.Warn(fmt.Sprintf("haproxy: could not remove firewall rule for port %d (may not exist)", port.Number))
		}
	}

	result, err = p.Exec.Run(ctx, "sudo", "firewall-cmd", "--reload")
	if err != nil || result.ExitCode != 0 {
		p.Log.Warn("haproxy: could not reload firewall")
	}

	// Remove the VIP secondary IP from the bastion so traffic routes to the
	// real kube-vip holder instead of being handled locally.
	if vip != "" {
		iface, ifaceErr := netutil.GetDefaultInterface(ctx)
		if ifaceErr != nil {
			p.Log.Warn(fmt.Sprintf("haproxy: could not detect default interface for VIP removal: %v", ifaceErr))
		} else {
			p.Log.Info(fmt.Sprintf("haproxy: removing vip %s from %s", vip, iface))
			if rmErr := netutil.RemoveSecondaryIP(ctx, vip, iface); rmErr != nil {
				p.Log.Warn(fmt.Sprintf("haproxy: could not remove vip %s from %s: %v", vip, iface, rmErr))
			}
		}

		// Wait for the API to become reachable via the VIP now that the
		// bastion no longer intercepts the traffic.
		p.Log.Info("haproxy: verifying api reachable via vip after teardown")
		if waitErr := system.WaitForWithTimeout(ctx, "haproxy", "api-via-vip", func() bool {
			if ctx.Err() != nil {
				return false
			}
			healthURL := fmt.Sprintf("https://%s:6443/healthz", vip)
			r, _ := p.Exec.Run(ctx, "curl", "-sk", "--connect-timeout", "5", healthURL)
			return r != nil && r.ExitCode == 0 && strings.TrimSpace(r.Stdout) == "ok"
		}, DefaultKubeVIPVIPTimeout, p.Log); waitErr != nil {
			return fmt.Errorf("api not reachable via vip %s after haproxy removal: %w", vip, waitErr)
		}
		p.Log.Info("haproxy: api confirmed reachable via vip")

		// Also verify via hostname -- removing the secondary IP can transiently
		// restart the local DNS forwarder, causing hostname resolution to lag.
		p.Log.Info("haproxy: verifying api reachable via hostname after teardown")
		if waitErr := system.WaitForWithTimeout(ctx, "haproxy", "api-via-hostname", func() bool {
			if ctx.Err() != nil {
				return false
			}
			r, _ := p.Exec.Run(ctx, "oc", "get", "--raw", "/healthz")
			return r != nil && r.ExitCode == 0 && strings.TrimSpace(r.Stdout) == "ok"
		}, DefaultKubeVIPVIPTimeout, p.Log); waitErr != nil {
			return fmt.Errorf("api not reachable via hostname after haproxy removal: %w", waitErr)
		}
		p.Log.Info("haproxy: api confirmed reachable via hostname")
	}

	return nil
}
