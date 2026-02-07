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

// Default timeouts for verification operations.
const (
	DefaultKubeVIPDaemonSetTimeout = 5 * time.Minute
	DefaultKubeVIPVIPTimeout       = 2 * time.Minute
)

// ClusterHealthResult contains the results of cluster health verification.
type ClusterHealthResult struct {
	DegradedOperators int
	ReadyNodes        int
	TotalNodes        int
}

// VerifyClusterHealth checks cluster operator status and node readiness.
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
				p.LogWarn(fmt.Sprintf("cluster: operator %s is degraded", fields[0]))
			}
		}
	}

	if result.DegradedOperators > 0 {
		p.LogWarn(fmt.Sprintf("cluster: %d operators are degraded", result.DegradedOperators))
	} else {
		p.LogInfo("cluster: all operators are healthy")
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

	p.LogInfo(fmt.Sprintf("cluster: %d/%d nodes are ready", result.ReadyNodes, result.TotalNodes))

	return result, nil
}

// ═══════════════════════════════════════════════════════════════════════════════
// KUBE-VIP VERIFICATION
// ═══════════════════════════════════════════════════════════════════════════════

// VerifyKubeVIP verifies that kube-vip is running and the VIP is responding.
// Returns the VIP address if successful.
func (p *Phase) VerifyKubeVIP(ctx context.Context, cfg *config.Config, opts Options) (string, error) {
	vip := netutil.DeriveVIPFromStaticIP(cfg.Networking.StaticIP.Start)
	if vip == "" {
		return "", fmt.Errorf("failed to derive VIP from static IP start: %s", cfg.Networking.StaticIP.Start)
	}

	p.LogInfo(fmt.Sprintf("kubevip: checking vip %s", vip))

	if err := p.waitForKubeVIPDaemonSet(ctx); err != nil {
		return "", err
	}

	if err := p.waitForKubeVIPPing(ctx, vip); err != nil {
		return "", err
	}

	if err := p.verifyKubeVIPAPIHealth(ctx, vip); err != nil {
		return "", err
	}

	return vip, nil
}

// waitForKubeVIPDaemonSet waits for the kube-vip DaemonSet to have at least one ready pod.
func (p *Phase) waitForKubeVIPDaemonSet(ctx context.Context) error {
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
	}, DefaultKubeVIPDaemonSetTimeout); err != nil {
		return utils.WrapError("kube-vip daemonset not ready", err)
	}

	result, _ := p.Exec.Run(ctx, "oc", "get", "daemonset", "-n", "kube-system", "kube-vip",
		"-o", "jsonpath={.status.numberReady}")
	if result != nil && result.ExitCode == 0 {
		p.LogInfo(fmt.Sprintf("kubevip: daemonset running (%s pods ready)", strings.TrimSpace(result.Stdout)))
	}

	return nil
}

// waitForKubeVIPPing waits for the VIP to respond to ping.
func (p *Phase) waitForKubeVIPPing(ctx context.Context, vip string) error {
	if err := system.WaitForWithTimeout(ctx, "kubevip", "ping", func() bool {
		if ctx.Err() != nil {
			return false
		}
		result, _ := p.Exec.Run(ctx, "ping", "-c", "1", "-W", "2", vip)
		return result != nil && result.ExitCode == 0
	}, DefaultKubeVIPVIPTimeout); err != nil {
		return utils.WrapError(fmt.Sprintf("vip %s is not responding to ping", vip), err)
	}

	p.LogInfo(fmt.Sprintf("kubevip: vip %s is reachable", vip))
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

	p.LogInfo(fmt.Sprintf("kubevip: api server responding at %s", healthURL))
	return nil
}

