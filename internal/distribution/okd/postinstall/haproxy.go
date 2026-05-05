package postinstall

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/firewall"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/httputil"
	"github.com/qxtaiba/okdctl/internal/netutil"
	"github.com/qxtaiba/okdctl/internal/system"
)

var (
	haproxyConfigPath = phase.DefaultHAProxyConfigPath
	haproxyHealthPort = phase.KubeAPIPort
	haproxyVIPTimeout = DefaultKubeVIPVIPTimeout
)

// RemoveHAProxy stops and disables HAProxy on the bastion. If vip is non-empty,
// the secondary IP is removed from the bastion's interface and the API is
// re-verified via the VIP after teardown to ensure kube-vip is handling traffic.
// clusterDir is the openshift-install output directory used to load the cluster CA.
func (p *Phase) RemoveHAProxy(ctx context.Context, vip, clusterDir string) error {
	if system.IsServiceActive(ctx, "haproxy") {
		p.Log.Info("haproxy: stopping service")
		if err := system.ManageService(ctx, system.ServiceStop, "haproxy", "haproxy service"); err != nil {
			p.Log.Warn("haproxy: stop failed", "err", err)
		}
	}
	if system.IsServiceEnabled(ctx, "haproxy") {
		p.Log.Info("haproxy: disabling service")
		if err := system.ManageService(ctx, system.ServiceDisable, "haproxy", "haproxy service"); err != nil {
			p.Log.Warn("haproxy: disable failed", "err", err)
		}
	}

	p.Log.Info("haproxy: removing configuration")
	if err := os.RemoveAll(haproxyConfigPath); err != nil {
		p.Log.Warn("haproxy: failed to remove config", "err", err)
	}

	if err := firewall.RemoveRules(ctx, firewall.HAProxyFrontendPorts(), true, p.Log); err != nil {
		p.Log.Warn("haproxy: firewall cleanup incomplete", "err", err)
	}

	if vip != "" {
		vipRemoved := false
		iface, ifaceErr := netutil.GetDefaultInterface(ctx)
		if ifaceErr != nil {
			p.Log.Warn("haproxy: could not detect default interface for VIP removal", "err", ifaceErr)
		} else {
			p.Log.Info("haproxy: removing vip", "vip", vip, "iface", iface)
			if rmErr := netutil.RemoveSecondaryIP(ctx, vip, iface); rmErr != nil {
				p.Log.Warn("haproxy: could not remove vip", "vip", vip, "iface", iface, "err", rmErr)
			} else {
				vipRemoved = true
			}
		}

		p.Log.Info("haproxy: verifying api reachable via vip after teardown")
		kubeconfigPath := filepath.Join(clusterDir, "auth", "kubeconfig")
		var healthClient *http.Client
		pool, caErr := httputil.KubeconfigCAPool(kubeconfigPath)
		if caErr != nil {
			return &errtypes.ClusterError{Msg: "kubeconfig CA unavailable; cannot verify api via vip", Err: caErr}
		}
		healthClient = httputil.NewWithCA(pool, 5*time.Second)
		healthURL := fmt.Sprintf("https://%s:%d/healthz", vip, haproxyHealthPort)
		if waitErr := system.WaitForWithTimeout(ctx, "haproxy", "api-via-vip", func() bool {
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
			return &errtypes.NetworkError{Msg: fmt.Sprintf("api not reachable via vip %s after haproxy removal", vip), Err: waitErr}
		}
		if !vipRemoved {
			p.Log.Warn("haproxy: api is reachable but vip was not removed from bastion — traffic may still route through haproxy")
		} else {
			p.Log.Info("haproxy: api confirmed reachable via vip")
		}

		// Removing the secondary IP can transiently restart the local DNS
		// forwarder, causing hostname resolution to lag — verify separately.
		p.Log.Info("haproxy: verifying api reachable via hostname after teardown")
		if waitErr := system.WaitForWithTimeout(ctx, "haproxy", "api-via-hostname", func() bool {
			r, _ := p.Exec.Run(ctx, "oc", "get", "--raw", "/healthz")
			return r.ExitCode == 0 && strings.TrimSpace(r.Stdout) == healthzOKBody
		}, haproxyVIPTimeout, p.Log); waitErr != nil {
			return &errtypes.ClusterError{Msg: "api not reachable via hostname after haproxy removal", Err: waitErr}
		}
		p.Log.Info("haproxy: api confirmed reachable via hostname")
	}

	return nil
}
