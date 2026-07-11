package postinstall

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

func TestWaitForRouterGone(t *testing.T) {
	installFakeOCForIngress(t)
	p := newIngressTestPhase(t)

	t.Run("already gone returns immediately", func(t *testing.T) {
		if err := p.waitForRouterGone(context.Background(), "default", 5*time.Second); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("gone after N polls", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			counter := filepath.Join(t.TempDir(), "counter")
			t.Setenv("OC_DEPLOY_CALL_FILE", counter)
			t.Setenv("OC_DEPLOY_GONE_AT", "3")

			if err := p.waitForRouterGone(context.Background(), "default", time.Minute); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	})

	t.Run("never gone times out with ClusterError", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			counter := filepath.Join(t.TempDir(), "counter")
			t.Setenv("OC_DEPLOY_CALL_FILE", counter)
			t.Setenv("OC_DEPLOY_GONE_AT", "9999")

			err := p.waitForRouterGone(context.Background(), "default", 12*time.Second)
			if err == nil {
				t.Fatal("expected timeout error")
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("err = %v; want context.DeadlineExceeded in chain", err)
			}
			var ce *errtypes.ClusterError
			if !errors.As(err, &ce) {
				t.Errorf("err is %T; want *errtypes.ClusterError so it maps to exit 4", err)
			}
		})
	})
}

func TestWaitForServiceLB(t *testing.T) {
	installFakeOCForIngress(t)
	p := newIngressTestPhase(t)

	t.Run("ip available on first probe", func(t *testing.T) {
		t.Setenv("OC_SVC_IP_DEFAULT", "10.0.0.40")

		ip, err := p.waitForServiceLB(context.Background(), "router-default", &Options{Timeout: 5 * time.Second})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ip != "10.0.0.40" {
			t.Errorf("ip = %q; want 10.0.0.40", ip)
		}
	})

	t.Run("pending ip resolves after N polls", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			counter := filepath.Join(t.TempDir(), "counter")
			t.Setenv("OC_SVC_CALL_FILE", counter)
			t.Setenv("OC_SVC_READY_AT", "3")
			t.Setenv("OC_SVC_IP_DEFAULT", "10.0.0.40")

			ip, err := p.waitForServiceLB(context.Background(), "router-default", &Options{Timeout: 5 * time.Minute})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ip != "10.0.0.40" {
				t.Errorf("ip = %q; want 10.0.0.40 once the pending ip is assigned", ip)
			}
		})
	})

	t.Run("ip never assigned times out", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			_, err := p.waitForServiceLB(context.Background(), "router-default", &Options{Timeout: 100 * time.Second})
			if err == nil {
				t.Fatal("expected timeout error")
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("err = %v; want context.DeadlineExceeded in chain", err)
			}
			var ce *errtypes.ClusterError
			if !errors.As(err, &ce) {
				t.Errorf("err is %T; want *errtypes.ClusterError", err)
			}
		})
	})

	t.Run("zero timeout falls back to default lb timeout", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			_, err := p.waitForServiceLB(context.Background(), "router-default", &Options{})
			if err == nil {
				t.Fatal("expected timeout error")
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("err = %v; want context.DeadlineExceeded in chain", err)
			}
		})
	})
}

func TestCollectLBEntries(t *testing.T) {
	installFakeOCForIngress(t)
	p := newIngressTestPhase(t)

	lbIC := func(name, domain string, converted bool) ingressControllerInfo {
		return ingressControllerInfo{Name: name, Domain: domain, Strategy: strategyLoadBalancer, converted: converted}
	}

	t.Run("mixes lb, converted, and hostnetwork entries", func(t *testing.T) {
		t.Setenv("OC_SVC_IP_DEFAULT", "10.0.0.40")
		t.Setenv("OC_SVC_IP_OTHER", "10.0.0.41")

		lbICs := []ingressControllerInfo{
			lbIC("default", "apps.a.test", false),
			lbIC("custom", "apps.b.test", true),
		}
		hostICs := []ingressControllerInfo{
			{Name: "internal", Domain: "apps.c.test", Strategy: strategyHostNetwork},
		}

		entries, customDomains, defaultAppsIP, err := p.collectLBEntries(
			context.Background(), lbICs, hostICs, "10.0.0.1", &Options{Timeout: 5 * time.Second})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if defaultAppsIP != "10.0.0.40" {
			t.Errorf("defaultAppsIP = %q; want 10.0.0.40", defaultAppsIP)
		}
		if len(entries) != 3 {
			t.Fatalf("entries = %d; want 3", len(entries))
		}
		if entries[0].Name != "default" || entries[0].LBIP != "10.0.0.40" || entries[0].Converted {
			t.Errorf("default entry = %+v; want LBIP 10.0.0.40 and Converted=false", entries[0])
		}
		if entries[1].Name != "custom" || entries[1].LBIP != "10.0.0.41" || !entries[1].Converted {
			t.Errorf("custom entry = %+v; want LBIP 10.0.0.41 and Converted=true", entries[1])
		}
		if entries[2].Name != "internal" || entries[2].LBIP != "10.0.0.1" || !entries[2].HostNetwork {
			t.Errorf("hostnetwork entry = %+v; want bastion IP and HostNetwork=true", entries[2])
		}
		if len(customDomains) != 1 || customDomains[0].Domain != "apps.b.test" || customDomains[0].IP != "10.0.0.41" {
			t.Errorf("customDomains = %+v; want apps.b.test → 10.0.0.41 only", customDomains)
		}
	})

	t.Run("default controller without ip is fatal", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			lbICs := []ingressControllerInfo{lbIC("default", "apps.a.test", false)}

			_, _, _, err := p.collectLBEntries(
				context.Background(), lbICs, nil, "10.0.0.1", &Options{Timeout: 100 * time.Second})
			if err == nil {
				t.Fatal("expected error when router-default never gets an ip")
			}
			var ce *errtypes.ClusterError
			if !errors.As(err, &ce) {
				t.Fatalf("err is %T; want *errtypes.ClusterError", err)
			}
		})
	})

	t.Run("non-default controller without ip is skipped", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			t.Setenv("OC_SVC_IP_DEFAULT", "10.0.0.40")

			lbICs := []ingressControllerInfo{
				lbIC("custom", "apps.b.test", false),
				lbIC("default", "apps.a.test", false),
			}

			entries, customDomains, defaultAppsIP, err := p.collectLBEntries(
				context.Background(), lbICs, nil, "10.0.0.1", &Options{Timeout: 100 * time.Second})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(entries) != 1 || entries[0].Name != "default" {
				t.Fatalf("entries = %+v; want only the default controller", entries)
			}
			if defaultAppsIP != "10.0.0.40" {
				t.Errorf("defaultAppsIP = %q; want 10.0.0.40", defaultAppsIP)
			}
			if len(customDomains) != 0 {
				t.Errorf("customDomains = %+v; want none for a skipped controller", customDomains)
			}
		})
	})
}
