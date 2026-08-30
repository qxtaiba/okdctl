package cluster

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/testutil"
)

const healthyCephJSON = `{
  "quorum_names": ["a","b","c"],
  "monmap": {"mons":[{},{},{}]},
  "osdmap": {"num_osds":3,"num_up_osds":3,"num_in_osds":3},
  "pgmap": {"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}
}`

// installFakeOCCeph installs a PATH-shadow "oc": get emits $OC_PODS_JSON;
// exec emits $OC_CEPH_JSON, or fails when $OC_EXEC_FAIL is set.
func installFakeOCCeph(t *testing.T) (argvLog string) {
	t.Helper()
	testutil.InstallFakeBin(t, "oc", `#!/bin/sh
echo "$@" >> "$OC_ARGV_LOG"
case "$1" in
get) printf '%s' "$OC_PODS_JSON" ;;
exec)
  if [ -n "$OC_EXEC_FAIL" ]; then
    echo "ceph command crashed" >&2
    exit 1
  fi
  printf '%s' "$OC_CEPH_JSON"
  ;;
esac
exit 0
`)
	argvLog = filepath.Join(t.TempDir(), "argv.log")
	t.Setenv("OC_ARGV_LOG", argvLog)
	t.Setenv("OC_CEPH_JSON", healthyCephJSON)
	return argvLog
}

func TestCephHealthy_NotApplicable(t *testing.T) {
	tests := []struct {
		name     string
		podsJSON string
	}{
		{name: "no toolbox pod", podsJSON: `{"items":[]}`},
		{name: "unscheduled toolbox pod", podsJSON: `{"items":[
	  {"metadata":{"name":"tools-pending","namespace":"rook-ceph"},"spec":{}}
	]}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			installFakeOCCeph(t)
			t.Setenv("OC_PODS_JSON", tc.podsJSON)
			c := New(WithCLI("oc"), WithExecutor(executor.New()))

			h, err := c.CephHealthy(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if h.Applicable {
				t.Errorf("Applicable = true with %s; want false (skip gate)", tc.name)
			}
			if h.Healthy {
				t.Errorf("Healthy = true with %s; want false", tc.name)
			}
		})
	}
}

func TestCephHealthy_ExecsFirstScheduledPod(t *testing.T) {
	argvLog := installFakeOCCeph(t)
	t.Setenv("OC_PODS_JSON", `{"items":[
	  {"metadata":{"name":"tools-pending","namespace":"rook-ceph"},"spec":{}},
	  {"metadata":{"name":"tools-ready","namespace":"rook-ceph"},"spec":{"nodeName":"worker1"}}
	]}`)
	c := New(WithCLI("oc"), WithExecutor(executor.New()))

	h, err := c.CephHealthy(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !h.Applicable || !h.Healthy {
		t.Fatalf("want applicable+healthy from healthy ceph json; got %+v", h)
	}
	argv, err := os.ReadFile(argvLog)
	if err != nil {
		t.Fatalf("argv log not written: %v", err)
	}
	if !strings.Contains(string(argv), "exec -n rook-ceph tools-ready --") {
		t.Errorf("exec did not target the first scheduled pod; argv:\n%s", argv)
	}
	if strings.Contains(string(argv), "tools-pending --") {
		t.Errorf("exec targeted an unscheduled pod; argv:\n%s", argv)
	}
}

func TestCephHealthy_ExecFailureClosesGate(t *testing.T) {
	installFakeOCCeph(t)
	t.Setenv("OC_PODS_JSON", `{"items":[
	  {"metadata":{"name":"tools-ready","namespace":"rook-ceph"},"spec":{"nodeName":"worker1"}}
	]}`)
	t.Setenv("OC_EXEC_FAIL", "1")
	c := New(WithCLI("oc"), WithExecutor(executor.New()))

	h, err := c.CephHealthy(context.Background())
	if err == nil {
		t.Fatal("expected error for failed ceph exec")
	}
	var ce *errtypes.ClusterError
	if !errors.As(err, &ce) {
		t.Fatalf("err type = %T; want *errtypes.ClusterError", err)
	}
	if h.Healthy {
		t.Errorf("Healthy = true on exec failure; the gate must stay closed")
	}
}

// installFakeOCEtcd installs a PATH-shadow "oc" keyed by resource:
// clusteroperator→$OC_CO_JSON, pods→$OC_PODS_JSON, etcd→$OC_ETCD_JSON.
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
