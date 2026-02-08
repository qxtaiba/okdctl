package postinstall

import (
	"context"
	"fmt"
	"time"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/netutil"
)

type UpdateIngressOptions struct {
	RemoveHAProxy bool
	Timeout       time.Duration
}

type UpdateIngressResult struct {
	RouterLBIP     string
	CustomRouterIP string
	KubeVipIP      string
	HAProxyRemoved bool
}

// UpdateIngress waits for LoadBalancer IPs on the default and custom ingress
// routers, deploys production DNS pointing *.apps at the real LB IPs, and
// optionally removes HAProxy from the bastion.
func (p *Phase) UpdateIngress(ctx context.Context, cfg *config.Config, opts UpdateIngressOptions) (*UpdateIngressResult, error) {
	vip := netutil.DeriveVIPFromStaticIP(cfg.Networking.StaticIP.Start)
	if vip == "" {
		return nil, fmt.Errorf("failed to derive VIP from static IP start: %s", cfg.Networking.StaticIP.Start)
	}

	p.Log.Info("update-ingress: waiting for router-default LoadBalancer IP")

	postOpts := Options{
		Timeout: opts.Timeout,
	}
	if postOpts.Timeout == 0 {
		postOpts.Timeout = DefaultTimeout
	}

	routerIP, err := p.waitForDefaultRouterLB(ctx, postOpts)
	if err != nil {
		return nil, fmt.Errorf("router-default has no LoadBalancer IP: %w", err)
	}
	p.Log.Info(fmt.Sprintf("update-ingress: router-default LoadBalancer IP is %s", routerIP))

	var customRouterIP string
	if cfg.Networking.CustomDomain != "" {
		p.Log.Info(fmt.Sprintf("update-ingress: waiting for router-%s LoadBalancer IP", cfg.Cluster.Name))
		customRouterIP, err = p.waitForCustomRouterLB(ctx, cfg.Cluster.Name, postOpts)
		if err != nil {
			p.Log.Warn(fmt.Sprintf("update-ingress: custom router has no LoadBalancer IP: %v", err))
		} else {
			p.Log.Info(fmt.Sprintf("update-ingress: router-%s LoadBalancer IP is %s", cfg.Cluster.Name, customRouterIP))
		}
	}

	p.Log.Info("update-ingress: deploying production DNS with LoadBalancer IPs")
	if err := p.deployProductionDNS(ctx, cfg, routerIP, vip, cfg.Networking.CustomDomain, customRouterIP); err != nil {
		return nil, fmt.Errorf("failed to deploy production DNS: %w", err)
	}
	p.Log.Info(fmt.Sprintf("update-ingress: dns updated — *.apps → %s, api.* → %s", routerIP, vip))

	result := &UpdateIngressResult{
		RouterLBIP:     routerIP,
		CustomRouterIP: customRouterIP,
		KubeVipIP:      vip,
	}

	if opts.RemoveHAProxy {
		p.Log.Info("update-ingress: removing HAProxy from bastion")
		if err := p.RemoveHAProxy(ctx, vip); err != nil {
			p.Log.Warn(fmt.Sprintf("update-ingress: haproxy removal failed: %v", err))
		} else {
			result.HAProxyRemoved = true
			p.Log.Info("update-ingress: haproxy removed from bastion")
		}
	}

	return result, nil
}
