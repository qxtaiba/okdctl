package postinstall

import (
	"context"
	"fmt"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

// RemoveHAProxy stops and disables HAProxy on the bastion, removing it as the API load balancer.
// This should only be called after kube-vip has been verified as working.
func (p *Phase) RemoveHAProxy(ctx context.Context) error {
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

	return nil
}
