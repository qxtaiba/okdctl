package postinstall

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/firewall"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/httputil"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

var (
	haproxyConfigPath = phase.DefaultHAProxyConfigPath
	haproxyHealthPort = phase.KubeAPIPort
	haproxyVIPTimeout = DefaultKubeVIPVIPTimeout
)

// RemoveHAProxy stops and disables HAProxy on the bastion. If vip is non-empty,
// the API is verified via the VIP *before* any destructive operation to confirm
// kube-vip is already handling traffic; only then is haproxy stopped, its config
// backed up and removed, firewall rules cleared, and the secondary IP released.
// clusterDir is the openshift-install output directory used to load the cluster CA.
// Exported for the reserved okdctl haproxy subcommand space; the only in-tree
// caller today is finalizeIngress in update_ingress.go.
func (p *Phase) RemoveHAProxy(ctx context.Context, vip, clusterDir string) error {
	if vip != "" {
		kubeconfigPath := workspace.KubeconfigPath(clusterDir)
		pool, caErr := httputil.KubeconfigCAPool(kubeconfigPath)
		if caErr != nil {
			return &errtypes.ClusterError{Msg: "kubeconfig CA unavailable; cannot verify api via vip", Err: caErr}
		}
		healthClient := httputil.NewWithCA(pool, 5*time.Second)
		healthURL := fmt.Sprintf("https://%s:%d/healthz", vip, haproxyHealthPort)

		p.Log.Info("haproxy: pre-flight — verifying api reachable via vip before teardown")
		if waitErr := system.WaitForWithTimeout(ctx, "haproxy", "api-via-vip", func(ctx context.Context) bool {
			req, rErr := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, http.NoBody)
			if rErr != nil {
				return false
			}
			resp, rErr := healthClient.Do(req)
			if rErr != nil {
				return false
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			return resp.StatusCode == http.StatusOK && strings.TrimSpace(string(body)) == healthzOKBody
		}, haproxyVIPTimeout, p.Log); waitErr != nil {
			return &errtypes.ClusterError{Msg: fmt.Sprintf("api not reachable via vip %s; aborting haproxy removal", vip), Err: waitErr}
		}
		p.Log.Info("haproxy: pre-flight — api confirmed reachable via vip")

		// Verify hostname resolution before stopping haproxy so the bastion's
		// dnsmasq is confirmed to route api.* to the VIP rather than localhost.
		p.Log.Info("haproxy: pre-flight — verifying api reachable via hostname before teardown")
		if waitErr := system.WaitForWithTimeout(ctx, "haproxy", "api-via-hostname", func(ctx context.Context) bool {
			out, err := p.OcOutput(ctx, "get", "--raw", "/healthz")
			return err == nil && out == healthzOKBody
		}, haproxyVIPTimeout, p.Log); waitErr != nil {
			return &errtypes.ClusterError{Msg: "api not reachable via hostname; aborting haproxy removal", Err: waitErr}
		}
		p.Log.Info("haproxy: pre-flight — api confirmed reachable via hostname")
	}

	phase.StopAndDisableService(ctx, "haproxy", p.Log)

	backupPath := phase.HAProxyTimestampedBackupPath(haproxyConfigPath, time.Now())
	if err := system.CopyFile(haproxyConfigPath, backupPath); err != nil {
		p.Log.Warn("haproxy: could not back up config; removal will proceed without a recovery artifact", "err", err)
	} else {
		p.Log.Info("haproxy: config backed up", "path", backupPath)
	}

	p.Log.Info("haproxy: removing configuration")
	if err := os.RemoveAll(haproxyConfigPath); err != nil {
		p.Log.Warn("haproxy: failed to remove config", "err", err)
	}

	if err := firewall.New(firewall.WithLogger(p.Log)).RemoveRules(ctx, firewall.HAProxyFrontendPorts(), true); err != nil {
		p.Log.Warn("haproxy: firewall cleanup incomplete", "err", err)
	}

	phase.ReleaseVIP(ctx, vip, p.Log)

	return nil
}
