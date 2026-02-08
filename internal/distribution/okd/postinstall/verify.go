package postinstall

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/netutil"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

const (
	DefaultKubeVIPDaemonSetTimeout = 5 * time.Minute
	DefaultKubeVIPVIPTimeout       = 2 * time.Minute
)

type ClusterHealthResult struct {
	DegradedOperators int
	ReadyNodes        int
	TotalNodes        int
}

func (p *Phase) VerifyClusterHealth(ctx context.Context, opts Options) (*ClusterHealthResult, error) {
	result := &ClusterHealthResult{}

	cmdResult, err := p.Exec.Run(ctx, "oc", "get", "clusteroperators", "--no-headers")
	if err != nil {
		return nil, utils.WrapError("failed to get cluster operators", err)
	}

	lines := strings.Split(strings.TrimSpace(cmdResult.Stdout), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 5 {
			if fields[4] == "True" { // DEGRADED column
				result.DegradedOperators++
				p.Log.Warn(fmt.Sprintf("cluster: operator %s is degraded", fields[0]))
			}
		}
	}

	if result.DegradedOperators > 0 {
		p.Log.Warn(fmt.Sprintf("cluster: %d operators are degraded", result.DegradedOperators))
	} else {
		p.Log.Info("cluster: all operators are healthy")
	}

	cmdResult, err = p.Exec.Run(ctx, "oc", "get", "nodes", "--no-headers")
	if err != nil {
		return result, utils.WrapError("failed to get nodes", err)
	}

	nodeLines := strings.Split(strings.TrimSpace(cmdResult.Stdout), "\n")
	result.TotalNodes = len(nodeLines)
	for _, line := range nodeLines {
		if strings.Contains(line, "Ready") && !strings.Contains(line, "NotReady") {
			result.ReadyNodes++
		}
	}

	p.Log.Info(fmt.Sprintf("cluster: %d/%d nodes are ready", result.ReadyNodes, result.TotalNodes))

	return result, nil
}

// VerifyKubeVIP verifies that kube-vip is running and the VIP is responding.
// Returns the VIP address if successful.
func (p *Phase) VerifyKubeVIP(ctx context.Context, cfg *config.Config, opts Options) (string, error) {
	vip := netutil.DeriveVIPFromStaticIP(cfg.Networking.StaticIP.Start)
	if vip == "" {
		return "", fmt.Errorf("failed to derive VIP from static IP start: %s", cfg.Networking.StaticIP.Start)
	}

	p.Log.Info(fmt.Sprintf("kubevip: checking vip %s", vip))

	if err := p.waitForKubeVIPDaemonSet(ctx, opts); err != nil {
		return "", err
	}

	if err := p.waitForKubeVIPPing(ctx, vip, opts); err != nil {
		return "", err
	}

	if err := p.verifyKubeVIPAPIHealth(ctx, vip); err != nil {
		return "", err
	}

	return vip, nil
}

// waitForKubeVIPDaemonSet waits for the kube-vip DaemonSet to have at least one ready pod.
func (p *Phase) waitForKubeVIPDaemonSet(ctx context.Context, opts Options) error {
	timeout := opts.KubeVIPDaemonSetTimeout
	if timeout == 0 {
		timeout = DefaultKubeVIPDaemonSetTimeout
	}

	if err := system.WaitForWithTimeout(ctx, "kubevip", "daemonset", func() bool {
		if ctx.Err() != nil {
			return false
		}
		result, _ := p.Exec.Run(ctx, "oc", "get", "daemonset", "-n", "kube-system", "kube-vip",
			"-o", "jsonpath={.status.numberReady}")
		if result == nil || result.ExitCode != 0 {
			return false
		}
		return strings.TrimSpace(result.Stdout) != "" && strings.TrimSpace(result.Stdout) != "0"
	}, timeout, p.Log); err != nil {
		return utils.WrapError("kube-vip daemonset not ready", err)
	}

	result, _ := p.Exec.Run(ctx, "oc", "get", "daemonset", "-n", "kube-system", "kube-vip",
		"-o", "jsonpath={.status.numberReady}")
	if result != nil && result.ExitCode == 0 {
		p.Log.Info(fmt.Sprintf("kubevip: daemonset running (%s pods ready)", strings.TrimSpace(result.Stdout)))
	}

	return nil
}

// waitForKubeVIPPing waits for the VIP to respond to ping.
func (p *Phase) waitForKubeVIPPing(ctx context.Context, vip string, opts Options) error {
	timeout := opts.KubeVIPVIPTimeout
	if timeout == 0 {
		timeout = DefaultKubeVIPVIPTimeout
	}

	if err := system.WaitForWithTimeout(ctx, "kubevip", "ping", func() bool {
		if ctx.Err() != nil {
			return false
		}
		result, _ := p.Exec.Run(ctx, "ping", "-c", "1", "-W", "2", vip)
		return result != nil && result.ExitCode == 0
	}, timeout, p.Log); err != nil {
		return utils.WrapError(fmt.Sprintf("vip %s is not responding to ping", vip), err)
	}

	p.Log.Info(fmt.Sprintf("kubevip: vip %s is reachable", vip))
	return nil
}

// verifyKubeVIPAPIHealth verifies the API server responds via the VIP.
func (p *Phase) verifyKubeVIPAPIHealth(ctx context.Context, vip string) error {
	// -k skips cert verification (VIP may not be in cert SANs)
	healthURL := fmt.Sprintf("https://%s:6443/healthz", vip)
	result, err := p.Exec.Run(ctx, "curl", "-sk", "--connect-timeout", "5", healthURL)
	if err != nil {
		return utils.WrapError(fmt.Sprintf("failed to check api health at %s", healthURL), err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("api health check failed at %s: %s", healthURL, result.Stderr)
	}

	response := strings.TrimSpace(result.Stdout)
	if response != "ok" {
		return fmt.Errorf("api health check returned unexpected response: %s (expected 'ok')", response)
	}

	p.Log.Info(fmt.Sprintf("kubevip: api server responding at %s", healthURL))
	return nil
}

// verifyAPIHealthCheck performs a quick API health check via the cluster hostname.
// Uses oc get --raw /healthz which goes through the kubeconfig's server URL.
func (p *Phase) verifyAPIHealthCheck(ctx context.Context) error {
	result, err := p.Exec.Run(ctx, "oc", "get", "--raw", "/healthz")
	if err != nil {
		return utils.WrapError("api health check failed", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("api health check failed: %s", result.Stderr)
	}
	if strings.TrimSpace(result.Stdout) != "ok" {
		return fmt.Errorf("api returned unexpected health status: %s", strings.TrimSpace(result.Stdout))
	}
	return nil
}
