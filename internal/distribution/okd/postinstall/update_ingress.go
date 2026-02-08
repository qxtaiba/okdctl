package postinstall

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/netutil"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

const DefaultIngressLBTimeout = 10 * time.Minute

type UpdateIngressOptions struct {
	RemoveHAProxy bool
	Timeout       time.Duration
}

// IngressEntry represents a discovered IngressController and its LB IP.
type IngressEntry struct {
	Name   string // IngressController name (e.g., "default", "grappleberry")
	Domain string // Domain served (e.g., "apps.grappleberry.k8s.local", "grappleberry.xyz")
	LBIP   string // LoadBalancer IP assigned by MetalLB
}

type UpdateIngressResult struct {
	Entries        []IngressEntry
	KubeVipIP      string
	HAProxyRemoved bool
}

// UpdateIngress discovers all IngressControllers from the cluster, waits for
// their LoadBalancer IPs, deploys production DNS, and optionally removes HAProxy.
func (p *Phase) UpdateIngress(ctx context.Context, cfg *config.Config, opts UpdateIngressOptions) (*UpdateIngressResult, error) {
	vip := netutil.DeriveVIPFromStaticIP(cfg.Networking.StaticIP.Start)
	if vip == "" {
		return nil, fmt.Errorf("failed to derive VIP from static IP start: %s", cfg.Networking.StaticIP.Start)
	}

	postOpts := Options{
		Timeout: opts.Timeout,
	}
	if postOpts.Timeout == 0 {
		postOpts.Timeout = DefaultTimeout
	}

	controllers, err := p.discoverIngressControllers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to discover IngressControllers: %w", err)
	}

	if len(controllers) == 0 {
		return nil, fmt.Errorf("no IngressControllers found in the cluster")
	}

	var entries []IngressEntry
	var defaultAppsIP string

	for _, ic := range controllers {
		svcName := fmt.Sprintf("router-%s", ic.Name)
		p.Log.Info(fmt.Sprintf("update-ingress: waiting for %s LoadBalancer IP", svcName))

		ip, err := p.waitForServiceLB(ctx, svcName, postOpts)
		if err != nil {
			p.Log.Warn(fmt.Sprintf("update-ingress: %s has no LoadBalancer IP: %v", svcName, err))
			continue
		}

		p.Log.Info(fmt.Sprintf("update-ingress: %s LoadBalancer IP is %s", svcName, ip))
		entries = append(entries, IngressEntry{
			Name:   ic.Name,
			Domain: ic.Domain,
			LBIP:   ip,
		})

		if ic.Name == "default" {
			defaultAppsIP = ip
		}
	}

	if defaultAppsIP == "" {
		return nil, fmt.Errorf("router-default has no LoadBalancer IP — cannot update ingress DNS")
	}

	// Build custom domain args for deployProductionDNS.
	// The first non-default entry becomes the custom domain (template supports one).
	var customDomain, customRouterIP string
	for _, e := range entries {
		if e.Name != "default" && e.LBIP != "" {
			customDomain = e.Domain
			customRouterIP = e.LBIP
			break
		}
	}

	p.Log.Info("update-ingress: deploying production DNS with LoadBalancer IPs")
	if err := p.deployProductionDNS(ctx, cfg, defaultAppsIP, vip, customDomain, customRouterIP); err != nil {
		return nil, fmt.Errorf("failed to deploy production DNS: %w", err)
	}
	p.Log.Info(fmt.Sprintf("update-ingress: dns updated — *.apps → %s, api.* → %s", defaultAppsIP, vip))

	result := &UpdateIngressResult{
		Entries:   entries,
		KubeVipIP: vip,
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

type ingressControllerInfo struct {
	Name   string
	Domain string
}

// discoverIngressControllers queries the cluster for all IngressControllers
// and returns their names and domains.
func (p *Phase) discoverIngressControllers(ctx context.Context) ([]ingressControllerInfo, error) {
	result, err := p.Exec.Run(ctx, "oc", "get", "ingresscontroller",
		"-n", "openshift-ingress-operator",
		"-o", "json")
	if err != nil {
		return nil, fmt.Errorf("failed to query IngressControllers: %w", err)
	}
	if result.ExitCode != 0 {
		return nil, fmt.Errorf("oc get ingresscontroller failed: %s", result.Stderr)
	}

	var list struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Status struct {
				Domain string `json:"domain"`
			} `json:"status"`
		} `json:"items"`
	}

	if err := json.Unmarshal([]byte(result.Stdout), &list); err != nil {
		return nil, fmt.Errorf("failed to parse IngressController list: %w", err)
	}

	var controllers []ingressControllerInfo
	for _, item := range list.Items {
		controllers = append(controllers, ingressControllerInfo{
			Name:   item.Metadata.Name,
			Domain: item.Status.Domain,
		})
	}

	return controllers, nil
}

// waitForServiceLB polls a service in openshift-ingress until a LoadBalancer IP is assigned.
func (p *Phase) waitForServiceLB(ctx context.Context, svcName string, opts Options) (string, error) {
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = DefaultIngressLBTimeout
	}

	var ip string
	if err := system.WaitForWithTimeout(ctx, "ingress", svcName+" lb", func() bool {
		if ctx.Err() != nil {
			return false
		}
		result, _ := p.Exec.Run(ctx, "oc", "get", "svc", svcName,
			"-n", "openshift-ingress",
			"-o", "jsonpath={.status.loadBalancer.ingress[0].ip}")
		if result == nil || result.ExitCode != 0 {
			return false
		}
		candidate := strings.TrimSpace(result.Stdout)
		if candidate == "" {
			return false
		}
		ip = candidate
		return true
	}, timeout, p.Log); err != nil {
		return "", fmt.Errorf("%s did not receive a LoadBalancer IP within %v", svcName, timeout)
	}

	return ip, nil
}
