package postinstall

import (
	"context"
	"fmt"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

// RemoveHAProxy stops and disables HAProxy on the bastion, removing it as the API load balancer.
// If vip is non-empty, the secondary IP is removed from the bastion's interface and the API
// is re-verified via the VIP after teardown to ensure kube-vip is handling traffic.
func (p *Phase) RemoveHAProxy(ctx context.Context, vip string) error {
	p.LogInfo("haproxy: stopping service")
	result, err := p.Exec.Run(ctx, "sudo", "systemctl", "stop", "haproxy")
	if err != nil {
		return utils.WrapError("failed to stop haproxy", err)
	}
	if result.ExitCode != 0 {
		p.LogWarn(fmt.Sprintf("haproxy: stop returned non-zero exit code: %s", result.Stderr))
	}

	p.LogInfo("haproxy: disabling service")
	result, err = p.Exec.Run(ctx, "sudo", "systemctl", "disable", "haproxy")
	if err != nil {
		return utils.WrapError("failed to disable haproxy", err)
	}
	if result.ExitCode != 0 {
		p.LogWarn(fmt.Sprintf("haproxy: disable returned non-zero exit code: %s", result.Stderr))
	}

	p.LogInfo("haproxy: removing configuration")
	_, err = p.Exec.Run(ctx, "sudo", "rm", "-f", "/etc/haproxy/haproxy.cfg")
	if err != nil {
		p.LogWarn(fmt.Sprintf("haproxy: failed to remove config: %v", err))
	}

	ports := []string{"6443", "22623", "80", "443"}
	for _, port := range ports {
		p.LogInfo(fmt.Sprintf("haproxy: removing firewall rule for port %s", port))
		result, err = p.Exec.Run(ctx, "sudo", "firewall-cmd", "--permanent", "--remove-port="+port+"/tcp")
		if err != nil || result.ExitCode != 0 {
			p.LogWarn(fmt.Sprintf("haproxy: could not remove firewall rule for port %s (may not exist)", port))
		}
	}

	result, err = p.Exec.Run(ctx, "sudo", "firewall-cmd", "--reload")
	if err != nil || result.ExitCode != 0 {
		p.LogWarn("haproxy: could not reload firewall")
	}

	// Remove the VIP secondary IP from the bastion so traffic routes to the
	// real kube-vip holder instead of being handled locally.
	if vip != "" {
		iface, ifaceErr := system.GetDefaultInterface(ctx)
		if ifaceErr != nil {
			p.LogWarn(fmt.Sprintf("haproxy: could not detect default interface for VIP removal: %v", ifaceErr))
		} else {
			p.LogInfo(fmt.Sprintf("haproxy: removing vip %s from %s", vip, iface))
			if rmErr := system.RemoveSecondaryIP(ctx, vip, iface); rmErr != nil {
				p.LogWarn(fmt.Sprintf("haproxy: could not remove vip %s from %s: %v", vip, iface, rmErr))
			}
		}

		// Wait for the API to become reachable via the VIP now that the
		// bastion no longer intercepts the traffic.
		p.LogInfo("haproxy: verifying api reachable via vip after teardown")
		if waitErr := system.WaitForWithTimeout(ctx, "haproxy", "api-via-vip", func() bool {
			if ctx.Err() != nil {
				return false
			}
			healthURL := fmt.Sprintf("https://%s:6443/healthz", vip)
			r, _ := p.Exec.Run(ctx, "curl", "-sk", "--connect-timeout", "5", healthURL)
			return r != nil && r.ExitCode == 0 && strings.TrimSpace(r.Stdout) == "ok"
		}, DefaultKubeVIPVIPTimeout); waitErr != nil {
			return fmt.Errorf("api not reachable via vip %s after haproxy removal: %w", vip, waitErr)
		}
		p.LogInfo("haproxy: api confirmed reachable via vip")

		// Verify the API is also reachable via hostname (as oc/kubectl will use).
		// The nmcli device reapply above can restart the local DNS forwarder,
		// causing a transient window where hostname resolution fails even though
		// the raw-IP check above succeeds.
		p.LogInfo("haproxy: verifying api reachable via hostname after teardown")
		if waitErr := system.WaitForWithTimeout(ctx, "haproxy", "api-via-hostname", func() bool {
			if ctx.Err() != nil {
				return false
			}
			r, _ := p.Exec.Run(ctx, "oc", "get", "--raw", "/healthz")
			return r != nil && r.ExitCode == 0 && strings.TrimSpace(r.Stdout) == "ok"
		}, DefaultKubeVIPVIPTimeout); waitErr != nil {
			return fmt.Errorf("api not reachable via hostname after haproxy removal: %w", waitErr)
		}
		p.LogInfo("haproxy: api confirmed reachable via hostname")
	}

	return nil
}
