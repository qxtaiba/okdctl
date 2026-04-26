package postinstall

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/httputil"
	"github.com/qxtaiba/okdctl/internal/system"
)

// nodeList is a minimal view of `oc get nodes -o json` output — only the
// fields we need for readiness counting. A local struct keeps the parse
// decoupled from corev1 schema evolution; we'd need to pin a specific
// k8s.io/api version in lockstep with the OKD release anyway.
type nodeList struct {
	Items []struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Status struct {
			Conditions []struct {
				Type   phase.ConditionType   `json:"type"`
				Status phase.ConditionStatus `json:"status"`
			} `json:"conditions"`
		} `json:"status"`
	} `json:"items"`
}

// parseNodeReadiness returns (ready, total) from a `oc get nodes -o json`
// payload. A node is ready when it has a condition with type=Ready and
// status=True. Replaces the prior strings.Contains(line, "Ready") && !strings.
// Contains(line, "NotReady") text-parse which misclassified "SchedulingDisabled
// Ready" output and could not distinguish transient state flaps.
func parseNodeReadiness(payload []byte) (ready, total int, err error) {
	var n nodeList
	if err := json.Unmarshal(payload, &n); err != nil {
		return 0, 0, fmt.Errorf("parse node list json: %w", err)
	}
	for _, node := range n.Items {
		total++
		for _, cond := range node.Status.Conditions {
			if cond.Type == phase.ConditionTypeReady && cond.Status == phase.ConditionStatusTrue {
				ready++
				break
			}
		}
	}
	return ready, total, nil
}

// Default timeouts for kube-vip readiness checks.
const (
	DefaultKubeVIPDaemonSetTimeout = 5 * time.Minute
	DefaultKubeVIPVIPTimeout       = 2 * time.Minute
)

// healthzOKBody is the plain-text body kube-apiserver /healthz returns on success.
const healthzOKBody = "ok"

// ClusterHealthResult summarizes a cluster-health probe.
type ClusterHealthResult struct {
	DegradedOperators int
	ReadyNodes        int
	TotalNodes        int
}

// VerifyClusterHealth probes ClusterOperators and Nodes, returning a summary
// of degraded operators and ready node counts.
func (p *Phase) VerifyClusterHealth(ctx context.Context, _ *Options) (*ClusterHealthResult, error) {
	result := &ClusterHealthResult{}

	cmdResult, err := p.Exec.RunChecked(ctx, "oc", "get", "clusteroperators", "--no-headers")
	if err != nil {
		return nil, &errtypes.ClusterError{Msg: "failed to get cluster operators", Err: err}
	}

	for line := range strings.Lines(cmdResult.Stdout) {
		fields := strings.Fields(line)
		if len(fields) >= 5 {
			if phase.ConditionStatus(fields[4]) == phase.ConditionStatusTrue { // DEGRADED column
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

	cmdResult, err = p.Exec.RunChecked(ctx, "oc", "get", "nodes", "-o", "json")
	if err != nil {
		return result, &errtypes.ClusterError{Msg: "failed to get nodes", Err: err}
	}

	ready, total, err := parseNodeReadiness([]byte(cmdResult.Stdout))
	if err != nil {
		return result, &errtypes.ClusterError{Msg: "failed to parse node readiness", Err: err}
	}
	result.ReadyNodes = ready
	result.TotalNodes = total

	p.Log.Info("cluster: node readiness", "ready", result.ReadyNodes, "total", result.TotalNodes)

	return result, nil
}

// VerifyKubeVIP verifies that kube-vip is running and the VIP is responding.
// Returns the VIP address if successful.
func (p *Phase) VerifyKubeVIP(ctx context.Context, cfg *config.Config, opts *Options) (string, error) {
	vip, err := phase.ResolveClusterVIP(cfg)
	if err != nil {
		return "", &errtypes.ConfigError{Msg: "failed to resolve cluster VIP", Err: err}
	}

	p.Log.Info(fmt.Sprintf("kubevip: checking vip %s", vip))

	if err := p.waitForKubeVIPDaemonSet(ctx, opts); err != nil {
		return "", &errtypes.ClusterError{Msg: "kube-vip daemonset not ready", Err: err}
	}

	if err := p.waitForKubeVIPPing(ctx, vip, opts); err != nil {
		return "", &errtypes.ClusterError{Msg: "kube-vip vip not reachable", Err: err}
	}

	if err := p.verifyKubeVIPAPIHealthBootstrap(ctx, vip, phase.ClusterConfigDir(opts.WorkDir)); err != nil {
		return "", &errtypes.ClusterError{Msg: "kube-vip api health check failed", Err: err}
	}

	return vip, nil
}

// waitForKubeVIPDaemonSet waits for the kube-vip DaemonSet to have at least one ready pod.
func (p *Phase) waitForKubeVIPDaemonSet(ctx context.Context, opts *Options) error {
	timeout := opts.KubeVIPDaemonSetTimeout
	if timeout == 0 {
		timeout = DefaultKubeVIPDaemonSetTimeout
	}
	ready, err := p.OcPollOutput(ctx, "kubevip", "daemonset", timeout,
		func(v string) bool { return v != "" && v != "0" },
		"get", "daemonset", "-n", "kube-system", "kube-vip",
		"-o", "jsonpath={.status.numberReady}")
	if err != nil {
		return fmt.Errorf("kube-vip daemonset not ready: %w", err)
	}
	p.Log.Info(fmt.Sprintf("kubevip: daemonset running (%s pods ready)", ready))
	return nil
}

// waitForKubeVIPPing waits for the VIP to respond to ping.
func (p *Phase) waitForKubeVIPPing(ctx context.Context, vip string, opts *Options) error {
	timeout := opts.KubeVIPVIPTimeout
	if timeout == 0 {
		timeout = DefaultKubeVIPVIPTimeout
	}

	dialer := &net.Dialer{Timeout: 2 * time.Second}
	if err := system.WaitForWithTimeout(ctx, "kubevip", "port 6443", func() bool {
		conn, dErr := dialer.DialContext(ctx, "tcp", net.JoinHostPort(vip, "6443"))
		if dErr != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, timeout, p.Log); err != nil {
		return fmt.Errorf("vip %s is not accepting tcp:6443: %w", vip, err)
	}

	p.Log.Info(fmt.Sprintf("kubevip: vip %s is reachable", vip))
	return nil
}

// verifyKubeVIPAPIHealthBootstrap verifies the API server responds via the VIP
// during the bootstrap-to-kube-vip transition, before the VIP appears in the
// apiserver certificate SANs. Falls back to InsecureSkipVerify only when the
// kubeconfig CA is not yet available; callers at later phases must use a
// verified client instead.
func (p *Phase) verifyKubeVIPAPIHealthBootstrap(ctx context.Context, vip, clusterDir string) error {
	healthURL := fmt.Sprintf("https://%s:6443/healthz", vip)

	var client *http.Client
	kubeconfigPath := filepath.Join(clusterDir, "auth", "kubeconfig")
	pool, err := httputil.KubeconfigCAPool(kubeconfigPath)
	if err != nil {
		// kubeconfig CA unavailable — VIP not yet in apiserver SANs; skip verify.
		p.Log.Warn("verify: kubeconfig CA unavailable, TLS verification skipped", "err", err)
		client = httputil.NewInsecure(5 * time.Second)
	} else {
		client = httputil.NewWithCA(pool, 5*time.Second)
	}

	p.Log.Info(fmt.Sprintf("verify: checking vip health at %s", healthURL))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, http.NoBody)
	if err != nil {
		return fmt.Errorf("failed to build health request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to check api health at %s: %w", healthURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read health response: %w", err)
	}
	response := strings.TrimSpace(string(body))
	if response != healthzOKBody {
		return &errtypes.ClusterError{
			Msg: fmt.Sprintf("api health check returned unexpected response: %s (expected 'ok')", response),
		}
	}

	p.Log.Info(fmt.Sprintf("kubevip: api server responding at %s", healthURL))
	return nil
}

// verifyAPIHealthCheck performs a quick API health check via the cluster hostname.
// Uses oc get --raw /healthz which goes through the kubeconfig's server URL.
func (p *Phase) verifyAPIHealthCheck(ctx context.Context) error {
	result, err := p.Exec.RunChecked(ctx, "oc", "get", "--raw", "/healthz")
	if err != nil {
		return &errtypes.ClusterError{Msg: "api health check failed", Err: err}
	}
	if strings.TrimSpace(result.Stdout) != healthzOKBody {
		return &errtypes.ClusterError{Msg: fmt.Sprintf("api returned unexpected health status: %s", strings.TrimSpace(result.Stdout))}
	}
	return nil
}
