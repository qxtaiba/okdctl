package cluster

import (
	"context"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/testutil"
)

func installFakeOCEtcd(t *testing.T) {
	t.Helper()
	testutil.InstallFakeBin(t, "oc", `#!/bin/sh
case "$2" in
clusteroperator) printf '%s' "$OC_CO_JSON" ;;
pods) printf '%s' "$OC_PODS_JSON" ;;
etcd) printf '%s' "$OC_ETCD_JSON" ;;
*) echo "unexpected resource: $2" >&2; exit 2 ;;
esac
exit 0
`)
	t.Setenv("OC_CO_JSON", `{"status":{"conditions":[
	  {"type":"Available","status":"True"},
	  {"type":"Degraded","status":"False"},
	  {"type":"Progressing","status":"False"}]}}`)
	t.Setenv("OC_PODS_JSON", `{"items":[
	  {"status":{"conditions":[{"type":"Ready","status":"True"}]}},
	  {"status":{"conditions":[{"type":"Ready","status":"True"}]}},
	  {"status":{"conditions":[{"type":"Ready","status":"True"}]}}]}`)
	t.Setenv("OC_ETCD_JSON", `{"status":{"conditions":[]}}`)
}

func TestEtcdHealthy_AllProbesGreen(t *testing.T) {
	installFakeOCEtcd(t)
	c := New(WithCLI("oc"), WithExecutor(executor.New()))

	h, err := c.EtcdHealthy(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !h.Healthy || !h.OperatorOK || h.PodsReady != 3 || h.PodsTotal != 3 || h.RolloutInFlight {
		t.Errorf("want healthy 3/3 no-rollout; got %+v", h)
	}
}

func TestEtcdHealthy_UnavailableOperatorClosesGate(t *testing.T) {
	installFakeOCEtcd(t)
	t.Setenv("OC_CO_JSON", `{"status":{"conditions":[
	  {"type":"Available","status":"False"},
	  {"type":"Degraded","status":"True"}]}}`)
	c := New(WithCLI("oc"), WithExecutor(executor.New()))

	h, err := c.EtcdHealthy(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.Healthy {
		t.Fatal("Healthy = true with unavailable operator; the gate must stay closed")
	}
	if h.OperatorOK {
		t.Error("OperatorOK = true for Available=False Degraded=True")
	}
	if !strings.Contains(h.Reason, "operator") {
		t.Errorf("Reason = %q; want it to name the operator check", h.Reason)
	}
}

func TestEtcdHealthy_RolloutInFlightClosesGate(t *testing.T) {
	installFakeOCEtcd(t)
	t.Setenv("OC_ETCD_JSON", `{"status":{"conditions":[
	  {"type":"NodeInstallerProgressing","status":"True"}]}}`)
	c := New(WithCLI("oc"), WithExecutor(executor.New()))

	h, err := c.EtcdHealthy(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.Healthy || !h.RolloutInFlight {
		t.Errorf("want unhealthy rollout-in-flight; got %+v", h)
	}
}
