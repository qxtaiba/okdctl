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
	"github.com/qxtaiba/okdctl/internal/netutil"
	"github.com/qxtaiba/okdctl/internal/system"
)

// RemoveHAProxy stops and disables HAProxy on the bastion, removing it as the API load balancer.
// If vip is non-empty, the secondary IP is removed from the bastion's interface and the API
// is re-verified via the VIP after teardown to ensure kube-vip is handling traffic.
func (p *Phase) RemoveHAProxy(ctx context.Context, vip string) error {
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
	if err := os.RemoveAll(phase.DefaultHAProxyConfigPath); err != nil {
		p.Log.Warn("haproxy: failed to remove config", "err", err)
	}

	if err := firewall.RemoveRules(ctx, firewall.HAProxyFrontendPorts(), true, p.Log); err != nil {
		p.Log.Warn("haproxy: firewall cleanup incomplete", "err", err)
	}

	// Remove the VIP secondary IP from the bastion so traffic routes to the
	// real kube-vip holder instead of being handled locally.
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

		// Wait for the API to become reachable via the VIP now that the
		// bastion no longer intercepts the traffic.
		p.Log.Info("haproxy: verifying api reachable via vip after teardown")
		healthClient := httputil.NewInsecure(5 * time.Second)
		healthURL := fmt.Sprintf("https://%s:6443/healthz", vip)
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
			return resp.StatusCode == http.StatusOK && strings.TrimSpace(string(body)) == "ok"
		}, DefaultKubeVIPVIPTimeout, p.Log); waitErr != nil {
			return &errtypes.NetworkError{Msg: fmt.Sprintf("api not reachable via vip %s after haproxy removal", vip), Err: waitErr}
		}
		if !vipRemoved {
			p.Log.Warn("haproxy: api is reachable but vip was not removed from bastion — traffic may still route through haproxy")
		} else {
			p.Log.Info("haproxy: api confirmed reachable via vip")
		}

		// Also verify via hostname -- removing the secondary IP can transiently
		// restart the local DNS forwarder, causing hostname resolution to lag.
		p.Log.Info("haproxy: verifying api reachable via hostname after teardown")
		if waitErr := system.WaitForWithTimeout(ctx, "haproxy", "api-via-hostname", func() bool {
			r, _ := p.Exec.Run(ctx, "oc", "get", "--raw", "/healthz")
			return r.ExitCode == 0 && strings.TrimSpace(r.Stdout) == "ok"
		}, DefaultKubeVIPVIPTimeout, p.Log); waitErr != nil {
			return &errtypes.ClusterError{Msg: "api not reachable via hostname after haproxy removal", Err: waitErr}
		}
		p.Log.Info("haproxy: api confirmed reachable via hostname")
	}

	return nil
}
