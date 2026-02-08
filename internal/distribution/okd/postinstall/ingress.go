package postinstall

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

const DefaultIngressLBTimeout = 10 * time.Minute

// waitForDefaultRouterLB polls the router-default service in openshift-ingress
// until MetalLB assigns a LoadBalancer IP. Returns the IP or an error on timeout.
func (p *Phase) waitForDefaultRouterLB(ctx context.Context, opts Options) (string, error) {
	timeout := opts.IngressLBTimeout
	if timeout == 0 {
		timeout = DefaultIngressLBTimeout
	}

	var ip string
	if err := system.WaitForWithTimeout(ctx, "ingress", "router lb", func() bool {
		if ctx.Err() != nil {
			return false
		}
		result, _ := p.Exec.Run(ctx, "oc", "get", "svc", "router-default",
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
		return "", fmt.Errorf("router-default did not receive a LoadBalancer IP within %v", timeout)
	}

	return ip, nil
}

const DefaultCustomRouterLBTimeout = 2 * time.Minute

// waitForCustomRouterLB polls the router-<name> service in openshift-ingress
// until MetalLB assigns a LoadBalancer IP. Shorter timeout than the default router
// since MetalLB should already be running by this point.
func (p *Phase) waitForCustomRouterLB(ctx context.Context, name string, opts Options) (string, error) {
	timeout := DefaultCustomRouterLBTimeout
	svcName := fmt.Sprintf("router-%s", name)

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
