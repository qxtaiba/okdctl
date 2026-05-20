package postinstall

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/httputil"
	"github.com/qxtaiba/okdctl/internal/system"
)

type verifyCondition struct {
	Type   phase.ConditionType   `json:"type"`
	Status phase.ConditionStatus `json:"status"`
}

// clusterOperatorList is a minimal view of `oc get clusteroperators -o json`
// output. Decoupling from operator.openshift.io/v1 avoids pinning that schema
// in lockstep with each OKD release.
type clusterOperatorList struct {
	Items []struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Status struct {
			Conditions []verifyCondition `json:"conditions"`
		} `json:"status"`
	} `json:"items"`
}

// parseOperatorDegradation returns the names of ClusterOperators carrying a
// type=Degraded status=True condition. Replaces a positional fields[4] text
// parse that broke whenever oc adjusted column ordering between releases.
func parseOperatorDegradation(payload []byte) ([]string, error) {
	var co clusterOperatorList
	if err := json.Unmarshal(payload, &co); err != nil {
		return nil, fmt.Errorf("parse clusteroperator list json: %w", err)
	}
	var degraded []string
	for _, op := range co.Items {
		if slices.ContainsFunc(op.Status.Conditions, func(c verifyCondition) bool {
			return c.Type == phase.ConditionTypeDegraded && c.Status == phase.ConditionStatusTrue
		}) {
			degraded = append(degraded, op.Metadata.Name)
		}
	}
	return degraded, nil
}

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
			Conditions []verifyCondition `json:"conditions"`
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
		if slices.ContainsFunc(node.Status.Conditions, func(c verifyCondition) bool {
			return c.Type == phase.ConditionTypeReady && c.Status == phase.ConditionStatusTrue
		}) {
			ready++
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

	cmdResult, err := p.Exec.RunChecked(ctx, "oc", "get", "clusteroperators", "-o", "json")
	if err != nil {
		return nil, &errtypes.ClusterError{Msg: "failed to get cluster operators", Err: err}
	}

	degraded, err := parseOperatorDegradation([]byte(cmdResult.Stdout))
	if err != nil {
		return nil, &errtypes.ClusterError{Msg: "failed to parse cluster operator status", Err: err}
	}
	result.DegradedOperators = len(degraded)
	if result.DegradedOperators > 0 {
		p.Log.Warn("cluster: operators degraded", "count", result.DegradedOperators, "names", degraded)
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

	p.Log.Info("kubevip: checking vip", "vip", vip)

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
		return err
	}
	p.Log.Info("kubevip: daemonset running", "pods_ready", ready)
	return nil
}

// waitForKubeVIPPing waits for the VIP to respond to ping.
func (p *Phase) waitForKubeVIPPing(ctx context.Context, vip string, opts *Options) error {
	timeout := opts.KubeVIPVIPTimeout
	if timeout == 0 {
		timeout = DefaultKubeVIPVIPTimeout
	}

	dialer := &net.Dialer{Timeout: 2 * time.Second}
	if err := system.WaitForWithTimeout(ctx, "kubevip", "port "+strconv.Itoa(phase.KubeAPIPort), func(context.Context) bool {
		conn, dErr := dialer.DialContext(ctx, "tcp", net.JoinHostPort(vip, strconv.Itoa(phase.KubeAPIPort)))
		if dErr != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, timeout, p.Log); err != nil {
		return fmt.Errorf("vip %s is not accepting tcp:%d: %w", vip, phase.KubeAPIPort, err)
	}

	p.Log.Info("kubevip: vip is reachable", "vip", vip)
	return nil
}

// verifyKubeVIPAPIHealthBootstrap verifies the API server responds via the VIP
// during the bootstrap-to-kube-vip transition. It tries a CA-verified request
// first; falls back to InsecureSkipVerify only when the error is
// x509.HostnameError (VIP not yet in the apiserver SANs — expected during the
// 1-3 minute kube-vip cert re-issue window). All other TLS errors propagate.
func (p *Phase) verifyKubeVIPAPIHealthBootstrap(ctx context.Context, vip, clusterDir string) error {
	healthURL := fmt.Sprintf("https://%s:%d/healthz", vip, phase.KubeAPIPort)

	kubeconfigPath := filepath.Join(clusterDir, "auth", "kubeconfig")
	pool, caErr := httputil.KubeconfigCAPool(kubeconfigPath)
	if caErr != nil {
		return &errtypes.ClusterError{Msg: "kubeconfig CA unavailable for kube-vip api health check", Err: caErr}
	}

	p.Log.Info("verify: checking vip health", "url", healthURL)

	doRequest := func(client *http.Client) (string, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, http.NoBody)
		if err != nil {
			return "", fmt.Errorf("failed to build health request: %w", err)
		}
		resp, err := client.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to read health response: %w", err)
		}
		return strings.TrimSpace(string(body)), nil
	}

	response, err := doRequest(httputil.NewWithCA(pool, 5*time.Second))
	if err != nil {
		var hostnameErr x509.HostnameError
		if !errors.As(err, &hostnameErr) {
			return &errtypes.ClusterError{Msg: fmt.Sprintf("api health check at %s", healthURL), Err: err}
		}
		// VIP not yet in apiserver SANs — transient during kube-vip cert re-issue; retry insecure.
		p.Log.Warn("verify: vip not in apiserver sans yet, retrying without tls verification", "vip", vip)
		response, err = doRequest(httputil.NewInsecure(5 * time.Second))
		if err != nil {
			return &errtypes.ClusterError{Msg: fmt.Sprintf("api health check at %s", healthURL), Err: err}
		}
	}

	if response != healthzOKBody {
		return &errtypes.ClusterError{
			Msg: fmt.Sprintf("api health check returned unexpected response: %s (expected 'ok')", response),
		}
	}

	p.Log.Info("kubevip: api server responding", "url", healthURL)
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
