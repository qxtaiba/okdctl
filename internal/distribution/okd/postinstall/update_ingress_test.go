package postinstall

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/templates"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/testutil"
)

func TestBuildLBIngressController_PreservesFields(t *testing.T) {
	cases := []struct {
		name          string
		icJSON        json.RawMessage
		hasReplicas   bool
		hasCert       bool
		hasRoute      bool
		hasAdmit      bool
		hasNode       bool
		wantDomain    string
		wantReplicas  int32
		wantNamespace string
	}{
		{
			name: "all optional fields present",
			icJSON: json.RawMessage(`{
				"metadata":{"name":"custom","namespace":"openshift-ingress-operator"},
				"spec":{
					"domain":"apps.example.com",
					"replicas":3,
					"defaultCertificate":{"name":"my-cert"},
					"routeSelector":{"matchLabels":{"env":"prod"}},
					"routeAdmission":{"wildcardPolicy":"WildcardsDisallowed"},
					"nodePlacement":{"nodeSelector":{"matchLabels":{"node-role.kubernetes.io/worker":""}}}
				}
			}`),
			hasReplicas:   true,
			hasCert:       true,
			hasRoute:      true,
			hasAdmit:      true,
			hasNode:       true,
			wantDomain:    "apps.example.com",
			wantReplicas:  3,
			wantNamespace: "openshift-ingress-operator",
		},
		{
			name: "absent optional fields stay omitted",
			icJSON: json.RawMessage(`{
				"metadata":{"name":"minimal","namespace":"openshift-ingress-operator"},
				"spec":{"domain":"apps.minimal.test"}
			}`),
			wantDomain:    "apps.minimal.test",
			wantNamespace: "openshift-ingress-operator",
		},
		{
			name: "empty namespace defaults to openshift-ingress-operator",
			icJSON: json.RawMessage(`{
				"metadata":{"name":"default","namespace":""},
				"spec":{"domain":"apps.cluster.local"}
			}`),
			wantDomain:    "apps.cluster.local",
			wantNamespace: "openshift-ingress-operator",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ic := &ingressControllerInfo{RawJSON: tc.icJSON}
			out, err := buildLBIngressController(ic)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var parsed struct {
				Spec struct {
					EndpointPublishingStrategy struct {
						Type string `json:"type"`
					} `json:"endpointPublishingStrategy"`
					Domain             string           `json:"domain"`
					Replicas           *int32           `json:"replicas"`
					DefaultCertificate *json.RawMessage `json:"defaultCertificate"`
					RouteSelector      *json.RawMessage `json:"routeSelector"`
					RouteAdmission     *json.RawMessage `json:"routeAdmission"`
					NodePlacement      *json.RawMessage `json:"nodePlacement"`
				} `json:"spec"`
				Metadata struct {
					Namespace string `json:"namespace"`
				} `json:"metadata"`
			}
			if err := json.Unmarshal([]byte(out), &parsed); err != nil {
				t.Fatalf("output is not valid JSON: %v", err)
			}

			if parsed.Spec.EndpointPublishingStrategy.Type != "LoadBalancerService" {
				t.Errorf("Type = %q; want LoadBalancerService", parsed.Spec.EndpointPublishingStrategy.Type)
			}
			if parsed.Metadata.Namespace != tc.wantNamespace {
				t.Errorf("Namespace = %q; want %q", parsed.Metadata.Namespace, tc.wantNamespace)
			}
			if parsed.Spec.Domain != tc.wantDomain {
				t.Errorf("Domain = %q; want %q", parsed.Spec.Domain, tc.wantDomain)
			}

			checkOpt := func(name string, want bool, got bool) {
				if want && !got {
					t.Errorf("%s expected but absent", name)
				}
				if !want && got {
					t.Errorf("%s unexpected but present", name)
				}
			}
			checkOpt("Replicas", tc.hasReplicas, parsed.Spec.Replicas != nil)
			if tc.hasReplicas && (parsed.Spec.Replicas == nil || *parsed.Spec.Replicas != tc.wantReplicas) {
				t.Errorf("Replicas = %v; want %d", parsed.Spec.Replicas, tc.wantReplicas)
			}
			checkOpt("DefaultCertificate", tc.hasCert, parsed.Spec.DefaultCertificate != nil)
			checkOpt("RouteSelector", tc.hasRoute, parsed.Spec.RouteSelector != nil)
			checkOpt("RouteAdmission", tc.hasAdmit, parsed.Spec.RouteAdmission != nil)
			checkOpt("NodePlacement", tc.hasNode, parsed.Spec.NodePlacement != nil)
		})
	}
}

func TestBuildRollbackJSON_StripsServerFields(t *testing.T) {
	icJSON := json.RawMessage(`{
		"apiVersion":"operator.openshift.io/v1",
		"kind":"IngressController",
		"metadata":{
			"name":"default",
			"namespace":"openshift-ingress-operator",
			"creationTimestamp":"2024-01-01T00:00:00Z",
			"generation":3,
			"resourceVersion":"99999",
			"selfLink":"/apis/operator.openshift.io/v1/...",
			"uid":"aaaa-bbbb-cccc",
			"managedFields":[{"manager":"oc","operation":"Apply"}]
		},
		"spec":{"domain":"apps.cluster.local"},
		"status":{"availableReplicas":2}
	}`)

	ic := &ingressControllerInfo{RawJSON: icJSON}
	out, err := buildRollbackJSON(ic)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, field := range []string{"creationTimestamp", "generation", "resourceVersion", "uid", "selfLink", "managedFields"} {
		if strings.Contains(out, field) {
			t.Errorf("rollback output still contains %q:\n%s", field, out)
		}
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatalf("rollback output is not valid JSON: %v", err)
	}
	if _, ok := obj["status"]; ok {
		t.Error("status key must be stripped but is present")
	}
	if _, ok := obj["spec"]; !ok {
		t.Error("spec must survive the strip but is absent")
	}
	var meta map[string]json.RawMessage
	if err := json.Unmarshal(obj["metadata"], &meta); err != nil {
		t.Fatalf("metadata is not valid JSON after strip: %v", err)
	}
	for _, keep := range []string{"name", "namespace"} {
		if _, ok := meta[keep]; !ok {
			t.Errorf("metadata.%s must survive the strip but is absent", keep)
		}
	}
}

// installFakeOCForIngress installs a fake "oc" dispatching on $1/$2, controlled
// by OC_-prefixed env vars (see the script body for the full set).
func installFakeOCForIngress(t *testing.T) {
	t.Helper()
	//nolint:dupword // embedded sh: adjacent fi keywords are not prose
	script := `#!/bin/sh
if [ -n "${OC_ARGV_LOG:-}" ]; then echo "$*" >> "$OC_ARGV_LOG"; fi
case "$1" in
  delete)
    if [ -n "${OC_BACKUP_EXPECT:-}" ] && [ -n "${OC_BACKUP_SEEN:-}" ]; then
      if [ -f "$OC_BACKUP_EXPECT" ]; then echo yes > "$OC_BACKUP_SEEN"; else echo no > "$OC_BACKUP_SEEN"; fi
    fi
    if [ "${OC_DELETE_FAIL:-0}" = "1" ]; then echo "fake: delete failed" >&2; exit 1; fi
    exit 0
    ;;
  get)
    case "$2" in
      ingresscontroller)
        if [ "${OC_IC_LIST_FAIL:-0}" = "1" ]; then echo "fake: discovery failed" >&2; exit 1; fi
        f="${OC_IC_LIST_FILE:-}"
        if [ -n "${OC_IC_CALL_FILE:-}" ]; then
          n=$(cat "$OC_IC_CALL_FILE" 2>/dev/null || echo 0)
          n=$((n + 1))
          echo "$n" > "$OC_IC_CALL_FILE"
          if [ "$n" -ge 2 ] && [ -n "${OC_IC_LIST_FILE2:-}" ]; then f="$OC_IC_LIST_FILE2"; fi
        fi
        if [ -n "$f" ]; then cat "$f"; fi
        exit 0
        ;;
      namespace)
        if [ "${OC_METALLB_NS:-0}" = "1" ]; then echo "metallb-system Active 5m"; fi
        exit 0
        ;;
      ipaddresspool)
        if [ "${OC_METALLB_POOL:-0}" = "1" ]; then echo "default-pool 5m"; fi
        exit 0
        ;;
      deployment)
        n=1
        if [ -n "${OC_DEPLOY_CALL_FILE:-}" ]; then
          n=$(cat "$OC_DEPLOY_CALL_FILE" 2>/dev/null || echo 0)
          n=$((n + 1))
          echo "$n" > "$OC_DEPLOY_CALL_FILE"
        fi
        if [ "$n" -lt "${OC_DEPLOY_GONE_AT:-1}" ]; then echo "router-fake 1/1 1 1 5m"; fi
        exit 0
        ;;
      svc)
        ip="${OC_SVC_IP_OTHER:-}"
        if [ "$3" = "router-default" ]; then ip="${OC_SVC_IP_DEFAULT:-}"; fi
        n=1
        if [ -n "${OC_SVC_CALL_FILE:-}" ]; then
          n=$(cat "$OC_SVC_CALL_FILE" 2>/dev/null || echo 0)
          n=$((n + 1))
          echo "$n" > "$OC_SVC_CALL_FILE"
        fi
        if [ "$n" -ge "${OC_SVC_READY_AT:-1}" ] && [ -n "$ip" ]; then echo "$ip"; fi
        exit 0
        ;;
      *) exit 0 ;;
    esac
    ;;
  create)
    n=1
    if [ -n "${OC_CALL_FILE:-}" ]; then
      n=$(cat "$OC_CALL_FILE" 2>/dev/null || echo 0)
      n=$((n + 1))
      echo "$n" > "$OC_CALL_FILE"
    fi
    if [ "$n" -eq 1 ] && [ "${OC_CREATE_SLEEP:-0}" = "1" ]; then exec sleep 5; fi
    if [ "$n" -ge 2 ] && [ -n "${OC_ROLLBACK_STDIN_LOG:-}" ]; then cat > "$OC_ROLLBACK_STDIN_LOG"; fi
    if [ "$n" -eq 1 ] && [ "${OC_CREATE_FAIL:-0}" = "1" ]; then echo "fake: create failed" >&2; exit 1; fi
    if [ "$n" -ge 2 ] && [ "${OC_ROLLBACK_FAIL:-0}" = "1" ]; then echo "fake: rollback failed" >&2; exit 1; fi
    exit 0
    ;;
  *) exit 0 ;;
esac
`
	testutil.InstallFakeBin(t, "oc", script)
}

// tempEnvFile points env var key at a fresh temp-dir file path and returns it.
func tempEnvFile(t *testing.T, key string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), key)
	t.Setenv(key, path)
	return path
}

func minimalIC(name string) *ingressControllerInfo {
	raw := `{"apiVersion":"operator.openshift.io/v1","kind":"IngressController",` +
		`"metadata":{"name":"` + name + `","namespace":"openshift-ingress-operator"},` +
		`"spec":{"domain":"apps.test.example.com"}}`
	return &ingressControllerInfo{
		Name:    name,
		Domain:  "apps.test.example.com",
		RawJSON: json.RawMessage(raw),
	}
}

func TestConvertToLoadBalancer_DeleteArgvTargetsICName(t *testing.T) {
	installFakeOCForIngress(t)

	argvLog := tempEnvFile(t, "OC_ARGV_LOG")
	tempEnvFile(t, "OC_CALL_FILE")

	p := newTestPhase(t)
	ic := minimalIC("custom")

	if err := p.convertToLoadBalancer(context.Background(), ic, 5*time.Second, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(argvLog)
	if err != nil {
		t.Fatalf("argv log not written: %v", err)
	}
	var deleteArgv string
	for _, l := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.HasPrefix(l, "delete ") {
			deleteArgv = l
			break
		}
	}
	if deleteArgv == "" {
		t.Fatalf("no delete argv recorded; log:\n%s", string(data))
	}
	if !strings.Contains(deleteArgv, "custom") {
		t.Errorf("delete argv %q does not name ic %q", deleteArgv, "custom")
	}
	if !strings.Contains(deleteArgv, "openshift-ingress-operator") {
		t.Errorf("delete argv %q does not target namespace openshift-ingress-operator", deleteArgv)
	}
}

func TestConvertToLoadBalancer_CreateFailure_RollbackIssued(t *testing.T) {
	installFakeOCForIngress(t)

	counter := tempEnvFile(t, "OC_CALL_FILE")
	rollbackStdin := tempEnvFile(t, "OC_ROLLBACK_STDIN_LOG")
	t.Setenv("OC_CREATE_FAIL", "1")

	p := newTestPhase(t)
	ic := minimalIC("default")

	if err := p.convertToLoadBalancer(context.Background(), ic, 5*time.Second, ""); err == nil {
		t.Fatal("expected error on create failure")
	}

	raw, err := os.ReadFile(counter)
	if err != nil {
		t.Fatalf("counter file not written: %v", err)
	}
	if strings.TrimSpace(string(raw)) != "2" {
		t.Errorf("oc create call count = %q; want 2 (replacement + rollback)", strings.TrimSpace(string(raw)))
	}

	wantJSON, buildErr := buildRollbackJSON(ic)
	if buildErr != nil {
		t.Fatalf("buildRollbackJSON: %v", buildErr)
	}
	gotStdin, readErr := os.ReadFile(rollbackStdin)
	if readErr != nil {
		t.Fatalf("rollback stdin log not written: %v", readErr)
	}
	if strings.TrimSpace(string(gotStdin)) != strings.TrimSpace(wantJSON) {
		t.Errorf("rollback stdin mismatch:\ngot:  %s\nwant: %s",
			strings.TrimSpace(string(gotStdin)), strings.TrimSpace(wantJSON))
	}
}

func TestConvertToLoadBalancer_BothCreateAndRollbackFail_ErrorNamesBoth(t *testing.T) {
	installFakeOCForIngress(t)

	tempEnvFile(t, "OC_CALL_FILE")
	t.Setenv("OC_CREATE_FAIL", "1")
	t.Setenv("OC_ROLLBACK_FAIL", "1")

	p := newTestPhase(t)
	ic := minimalIC("default")

	err := p.convertToLoadBalancer(context.Background(), ic, 5*time.Second, "")
	if err == nil {
		t.Fatal("expected error when both create and rollback fail")
	}
	msg := err.Error()
	if !strings.Contains(msg, "create") {
		t.Errorf("error %q does not mention create failure", msg)
	}
	if !strings.Contains(msg, "rollback") {
		t.Errorf("error %q does not mention rollback failure", msg)
	}
	wrapped := errors.Unwrap(err)
	if wrapped == nil {
		t.Fatal("expected wrapped error carrying both failures")
	}
	if !strings.Contains(wrapped.Error(), "fake: rollback failed") {
		t.Errorf("wrapped error %q does not carry redacted rollback stderr", wrapped.Error())
	}
}

func TestRestoreHAProxyBackup_PrefersTimestampedFallsBackToPristine(t *testing.T) {
	t.Run("prefers newest timestamped backup", func(t *testing.T) {
		cfg := filepath.Join(t.TempDir(), "haproxy.cfg")
		setHAProxyConfigPath(t, cfg)

		writes := map[string]string{
			cfg + phase.HAProxyBackupSuffix: "pristine",
			phase.HAProxyTimestampedBackupPath(cfg, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)): "old",
			phase.HAProxyTimestampedBackupPath(cfg, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)): "new",
		}
		for path, content := range writes {
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
		}

		if !newTestPhase(t).restoreHAProxyBackup() {
			t.Fatal("restoreHAProxyBackup returned false with backups present")
		}
		got, err := os.ReadFile(cfg)
		if err != nil {
			t.Fatalf("read restored config: %v", err)
		}
		if string(got) != "new" {
			t.Errorf("restored %q, want newest timestamped backup content \"new\"", got)
		}
	})

	t.Run("falls back to pristine snapshot", func(t *testing.T) {
		cfg := filepath.Join(t.TempDir(), "haproxy.cfg")
		setHAProxyConfigPath(t, cfg)

		if err := os.WriteFile(cfg+phase.HAProxyBackupSuffix, []byte("pristine"), 0o600); err != nil {
			t.Fatal(err)
		}

		if !newTestPhase(t).restoreHAProxyBackup() {
			t.Fatal("restoreHAProxyBackup returned false with pristine snapshot present")
		}
		got, err := os.ReadFile(cfg)
		if err != nil {
			t.Fatalf("read restored config: %v", err)
		}
		if string(got) != "pristine" {
			t.Errorf("restored %q, want pristine snapshot content", got)
		}
	})

	t.Run("returns false with no backups", func(t *testing.T) {
		cfg := filepath.Join(t.TempDir(), "haproxy.cfg")
		setHAProxyConfigPath(t, cfg)

		if newTestPhase(t).restoreHAProxyBackup() {
			t.Fatal("restoreHAProxyBackup returned true with no backup files")
		}
	})
}

func TestParseIngressStrategy(t *testing.T) {
	cases := []struct {
		in     string
		want   IngressStrategy
		wantOK bool
	}{
		{"HostNetwork", strategyHostNetwork, true},
		{"LoadBalancerService", strategyLoadBalancer, true},
		{"NodePortService", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		got, ok := parseIngressStrategy(tc.in)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("parseIngressStrategy(%q) = (%q, %v); want (%q, %v)", tc.in, got, ok, tc.want, tc.wantOK)
		}
	}
}

func hostNetworkIC(name string) *ingressControllerInfo {
	raw := `{"apiVersion":"operator.openshift.io/v1","kind":"IngressController",` +
		`"metadata":{"name":"` + name + `","namespace":"openshift-ingress-operator",` +
		`"resourceVersion":"12345","uid":"aaaa-bbbb"},` +
		`"spec":{"domain":"apps.test.example.com",` +
		`"endpointPublishingStrategy":{"type":"HostNetwork"}},` +
		`"status":{"availableReplicas":2}}`
	return &ingressControllerInfo{
		Name:     name,
		Domain:   "apps.test.example.com",
		Strategy: strategyHostNetwork,
		RawJSON:  json.RawMessage(raw),
	}
}

func TestAttemptRollback(t *testing.T) {
	installFakeOCForIngress(t)

	t.Run("create exit non-zero returns error with redacted stderr", func(t *testing.T) {
		tempEnvFile(t, "OC_CALL_FILE")
		t.Setenv("OC_CREATE_FAIL", "1")

		p := newTestPhase(t)
		err := p.attemptRollback(context.Background(), minimalIC("default"))
		if err == nil {
			t.Fatal("expected error on rollback create exit 1")
		}
		if !strings.Contains(err.Error(), "exited 1") {
			t.Errorf("err = %q; want exit code in message", err.Error())
		}
		if !strings.Contains(err.Error(), "fake: create failed") {
			t.Errorf("err = %q; want subprocess stderr carried through", err.Error())
		}
	})

	t.Run("unbuildable rollback json errors without calling oc", func(t *testing.T) {
		argvLog := tempEnvFile(t, "OC_ARGV_LOG")

		p := newTestPhase(t)
		ic := &ingressControllerInfo{Name: "broken", RawJSON: json.RawMessage(`{not json`)}
		if err := p.attemptRollback(context.Background(), ic); err == nil {
			t.Fatal("expected error for unparseable RawJSON")
		}
		if _, err := os.ReadFile(argvLog); err == nil {
			t.Error("oc was invoked despite rollback json build failure")
		}
	})
}

func TestHandleHostNetworkConversion_SkipPaths(t *testing.T) {
	installFakeOCForIngress(t)
	ics := func() []ingressControllerInfo { return []ingressControllerInfo{*hostNetworkIC("default")} }

	assertNoConversion := func(t *testing.T, argvLog string) {
		t.Helper()
		data, _ := os.ReadFile(argvLog)
		if strings.Contains(string(data), "delete ") {
			t.Errorf("conversion delete issued on a skip path; argv log:\n%s", string(data))
		}
	}

	t.Run("metallb namespace missing skips without error", func(t *testing.T) {
		argvLog := tempEnvFile(t, "OC_ARGV_LOG")

		p := newTestPhase(t)
		n, names, err := p.handleHostNetworkConversion(context.Background(), ics(),
			UpdateIngressOptions{ConfirmConversion: func([]string) bool { return true }}, &Options{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 0 || len(names) != 0 {
			t.Errorf("converted = %d names = %v; want 0 and empty", n, names)
		}
		assertNoConversion(t, argvLog)
	})

	t.Run("metallb pool missing skips without error", func(t *testing.T) {
		argvLog := tempEnvFile(t, "OC_ARGV_LOG")
		t.Setenv("OC_METALLB_NS", "1")

		p := newTestPhase(t)
		n, _, err := p.handleHostNetworkConversion(context.Background(), ics(),
			UpdateIngressOptions{ConfirmConversion: func([]string) bool { return true }}, &Options{})
		if err != nil || n != 0 {
			t.Errorf("converted = %d err = %v; want 0 and nil", n, err)
		}
		assertNoConversion(t, argvLog)
	})

	t.Run("metallb check transport error skips without error", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())

		p := newTestPhase(t)
		n, _, err := p.handleHostNetworkConversion(context.Background(), ics(),
			UpdateIngressOptions{ConfirmConversion: func([]string) bool { return true }}, &Options{})
		if err != nil || n != 0 {
			t.Errorf("converted = %d err = %v; want 0 and nil", n, err)
		}
	})

	t.Run("nil confirmation callback skips", func(t *testing.T) {
		argvLog := tempEnvFile(t, "OC_ARGV_LOG")
		t.Setenv("OC_METALLB_NS", "1")
		t.Setenv("OC_METALLB_POOL", "1")

		p := newTestPhase(t)
		n, _, err := p.handleHostNetworkConversion(context.Background(), ics(), UpdateIngressOptions{}, &Options{})
		if err != nil || n != 0 {
			t.Errorf("converted = %d err = %v; want 0 and nil", n, err)
		}
		assertNoConversion(t, argvLog)
	})

	t.Run("declined confirmation skips and passes ic names", func(t *testing.T) {
		argvLog := tempEnvFile(t, "OC_ARGV_LOG")
		t.Setenv("OC_METALLB_NS", "1")
		t.Setenv("OC_METALLB_POOL", "1")

		var gotNames []string
		p := newTestPhase(t)
		n, _, err := p.handleHostNetworkConversion(context.Background(), ics(),
			UpdateIngressOptions{ConfirmConversion: func(names []string) bool {
				gotNames = names
				return false
			}}, &Options{})
		if err != nil || n != 0 {
			t.Errorf("converted = %d err = %v; want 0 and nil", n, err)
		}
		if len(gotNames) != 1 || gotNames[0] != "default" {
			t.Errorf("confirmation callback received %v; want [default]", gotNames)
		}
		assertNoConversion(t, argvLog)
	})
}

func TestHandleHostNetworkConversion_ConvertsWhenConfirmed(t *testing.T) {
	installFakeOCForIngress(t)
	argvLog := tempEnvFile(t, "OC_ARGV_LOG")
	tempEnvFile(t, "OC_CALL_FILE")
	t.Setenv("OC_METALLB_NS", "1")
	t.Setenv("OC_METALLB_POOL", "1")

	p := newTestPhase(t)
	ics := []ingressControllerInfo{*hostNetworkIC("default"), *hostNetworkIC("custom")}
	n, names, err := p.handleHostNetworkConversion(context.Background(), ics,
		UpdateIngressOptions{ConfirmConversion: func([]string) bool { return true }},
		&Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 2 {
		t.Errorf("converted = %d; want 2", n)
	}
	for _, want := range []string{"default", "custom"} {
		if !names[want] {
			t.Errorf("names[%q] = false; want true", want)
		}
	}

	data, err := os.ReadFile(argvLog)
	if err != nil {
		t.Fatalf("argv log not written: %v", err)
	}
	var deletes, creates int
	for _, l := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		switch {
		case strings.HasPrefix(l, "delete "):
			deletes++
			if creates >= deletes {
				t.Errorf("create issued before delete for the same conversion; argv log:\n%s", string(data))
			}
		case strings.HasPrefix(l, "create "):
			creates++
		}
	}
	if deletes != 2 || creates != 2 {
		t.Errorf("deletes = %d creates = %d; want 2 and 2", deletes, creates)
	}
}

func TestHandleHostNetworkConversion_DeleteFailureStopsConversion(t *testing.T) {
	installFakeOCForIngress(t)
	tempEnvFile(t, "OC_CALL_FILE")
	t.Setenv("OC_METALLB_NS", "1")
	t.Setenv("OC_METALLB_POOL", "1")
	t.Setenv("OC_DELETE_FAIL", "1")

	p := newTestPhase(t)
	ics := []ingressControllerInfo{*hostNetworkIC("default")}
	n, _, err := p.handleHostNetworkConversion(context.Background(), ics,
		UpdateIngressOptions{ConfirmConversion: func([]string) bool { return true }},
		&Options{Timeout: 5 * time.Second})
	if err == nil {
		t.Fatal("expected error when oc delete fails")
	}
	if n != 0 {
		t.Errorf("converted = %d; want 0", n)
	}
	if !strings.Contains(err.Error(), `"default"`) {
		t.Errorf("err = %q; want failing controller named", err.Error())
	}
}

func TestHandleHostNetworkConversion_MidConversionFailureRestoresPriorStrategy(t *testing.T) {
	installFakeOCForIngress(t)
	tempEnvFile(t, "OC_CALL_FILE")
	rollbackStdin := tempEnvFile(t, "OC_ROLLBACK_STDIN_LOG")
	t.Setenv("OC_CREATE_FAIL", "1")
	t.Setenv("OC_METALLB_NS", "1")
	t.Setenv("OC_METALLB_POOL", "1")

	p := newTestPhase(t)
	ics := []ingressControllerInfo{*hostNetworkIC("default")}
	n, names, err := p.handleHostNetworkConversion(context.Background(), ics,
		UpdateIngressOptions{ConfirmConversion: func([]string) bool { return true }},
		&Options{Timeout: 5 * time.Second})
	if err == nil {
		t.Fatal("expected error when replacement create fails")
	}
	if n != 0 || len(names) != 0 {
		t.Errorf("converted = %d names = %v; want 0 and empty after failed conversion", n, names)
	}

	raw, readErr := os.ReadFile(rollbackStdin)
	if readErr != nil {
		t.Fatalf("rollback was not issued: %v", readErr)
	}
	var restored struct {
		Metadata struct {
			Name            string `json:"name"`
			ResourceVersion string `json:"resourceVersion"`
		} `json:"metadata"`
		Spec struct {
			EndpointPublishingStrategy struct {
				Type string `json:"type"`
			} `json:"endpointPublishingStrategy"`
		} `json:"spec"`
		Status json.RawMessage `json:"status"`
	}
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatalf("rollback payload is not valid JSON: %v", err)
	}
	if restored.Metadata.Name != "default" {
		t.Errorf("rollback restores %q; want default", restored.Metadata.Name)
	}
	if got := restored.Spec.EndpointPublishingStrategy.Type; got != "HostNetwork" {
		t.Errorf("rollback strategy = %q; want prior strategy HostNetwork", got)
	}
	if restored.Metadata.ResourceVersion != "" {
		t.Error("rollback payload still carries server-managed resourceVersion")
	}
	if len(restored.Status) != 0 {
		t.Error("rollback payload still carries status")
	}
}

func writeICList(t *testing.T, envVar string, items ...string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ic-list.json")
	list := `{"items":[` + strings.Join(items, ",") + `]}`
	if err := os.WriteFile(path, []byte(list), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envVar, path)
}

// stubDNSDeploys replaces the dns deploy seams, returning call captures;
// prodErr/bootErr inject failures.
func stubDNSDeploys(t *testing.T, prodErr, bootErr error) (prodAppsIPs *[]string, bootstrapCalls *int) {
	t.Helper()
	origProd := deployProductionDNSFn
	origBoot := deployBootstrapDNSFn
	t.Cleanup(func() {
		deployProductionDNSFn = origProd
		deployBootstrapDNSFn = origBoot
	})

	var appsIPs []string
	var boots int
	deployProductionDNSFn = func(_ context.Context, _ *config.Config, appsIP, _ string, _ []templates.DNSCustomDomain) error {
		appsIPs = append(appsIPs, appsIP)
		return prodErr
	}
	deployBootstrapDNSFn = func(context.Context, *config.Config) error {
		boots++
		return bootErr
	}
	return &appsIPs, &boots
}

func TestDiscoverIngressControllers(t *testing.T) {
	installFakeOCForIngress(t)

	t.Run("classifies strategies and skips unknown or unnamed", func(t *testing.T) {
		writeICList(
			t, "OC_IC_LIST_FILE",
			`{"metadata":{"name":"default"},"spec":{},"status":{"domain":"apps.a.test"}}`,
			`{"metadata":{"name":"lb"},"spec":{"endpointPublishingStrategy":{"type":"LoadBalancerService"}},"status":{"domain":"apps.b.test"}}`,
			`{"metadata":{"name":"nodeport"},"spec":{"endpointPublishingStrategy":{"type":"NodePortService"}},"status":{}}`,
			`{"metadata":{},"spec":{},"status":{}}`,
		)

		p := newTestPhase(t)
		got, err := p.discoverIngressControllers(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("controllers = %d; want 2 (unknown strategy and unnamed skipped)", len(got))
		}
		if got[0].Name != "default" || got[0].Strategy != strategyHostNetwork {
			t.Errorf("got[0] = %q/%q; want default with null strategy treated as HostNetwork", got[0].Name, got[0].Strategy)
		}
		if got[0].Domain != "apps.a.test" {
			t.Errorf("got[0].Domain = %q; want apps.a.test", got[0].Domain)
		}
		if got[1].Name != "lb" || got[1].Strategy != strategyLoadBalancer {
			t.Errorf("got[1] = %q/%q; want lb/LoadBalancerService", got[1].Name, got[1].Strategy)
		}
		if len(got[0].RawJSON) == 0 {
			t.Error("RawJSON not captured for rollback use")
		}
	})

	t.Run("oc failure propagates", func(t *testing.T) {
		t.Setenv("OC_IC_LIST_FAIL", "1")

		p := newTestPhase(t)
		if _, err := p.discoverIngressControllers(context.Background()); err == nil {
			t.Fatal("expected error when oc get fails")
		}
	})

	t.Run("unparseable list propagates", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "bad.json")
		if err := os.WriteFile(path, []byte(`{not json`), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("OC_IC_LIST_FILE", path)

		p := newTestPhase(t)
		if _, err := p.discoverIngressControllers(context.Background()); err == nil {
			t.Fatal("expected error for unparseable list")
		}
	})
}

func ingressTestConfig() *config.Config {
	return &config.Config{
		Cluster: config.ClusterConfig{Name: "okdctl-b24-test", Domain: "example.com"},
		Networking: config.NetworkingConfig{
			Bastion: config.BastionConfig{IP: "10.0.0.1", VIP: "10.0.0.5"},
		},
	}
}

func TestUpdateIngress_ErrorPaths(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T)
	}{
		{"discovery failure", func(t *testing.T) { t.Setenv("OC_IC_LIST_FAIL", "1") }},
		{"no controllers found", func(t *testing.T) { writeICList(t, "OC_IC_LIST_FILE") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			installFakeOCForIngress(t)
			tc.setup(t)

			p := newTestPhase(t)
			_, err := p.UpdateIngress(context.Background(), ingressTestConfig(), UpdateIngressOptions{})
			if err == nil {
				t.Fatalf("expected error on %s outside bootstrap dns state", tc.name)
			}
			var ce *errtypes.ClusterError
			if !errors.As(err, &ce) {
				t.Fatalf("err is %T; want *errtypes.ClusterError", err)
			}
		})
	}
}

func TestFinalizeIngress_DNSDeployFailurePropagates(t *testing.T) {
	appsIPs, _ := stubDNSDeploys(t, errors.New("dnsmasq restart failed"), nil)

	p := newTestPhase(t)
	_, err := p.finalizeIngress(context.Background(), ingressTestConfig(), UpdateIngressOptions{},
		nil, nil, "", "10.0.0.5", 0, 0)
	if err == nil {
		t.Fatal("expected error when production dns deploy fails")
	}
	var ce *errtypes.ClusterError
	if !errors.As(err, &ce) {
		t.Fatalf("err is %T; want *errtypes.ClusterError", err)
	}
	if len(*appsIPs) != 1 || (*appsIPs)[0] != "10.0.0.1" {
		t.Errorf("deploy called with apps IPs %v; want bastion fallback [10.0.0.1] when default has no lb ip", *appsIPs)
	}
}

func TestFinalizeIngress_SkipsHAProxyRemovalWithHostNetwork(t *testing.T) {
	_, bootstraps := stubDNSDeploys(t, nil, nil)

	p := newTestPhase(t)
	result, err := p.finalizeIngress(context.Background(), ingressTestConfig(),
		UpdateIngressOptions{RemoveHAProxy: true, WorkDir: t.TempDir()},
		nil, nil, "10.0.0.40", "10.0.0.5", 0, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.HAProxyRemoved {
		t.Error("HAProxyRemoved = true; removal must be skipped while hostnetwork controllers remain")
	}
	if *bootstraps != 0 {
		t.Errorf("bootstrap dns rollback ran %d times; want 0 when removal is skipped", *bootstraps)
	}
}

func TestFinalizeIngress_HAProxyRemovalFailureRollsBack(t *testing.T) {
	seedHAProxyBackup := func(t *testing.T) string {
		t.Helper()
		cfgPath := filepath.Join(t.TempDir(), "haproxy.cfg")
		setHAProxyConfigPath(t, cfgPath)
		backup := phase.HAProxyTimestampedBackupPath(cfgPath, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
		if err := os.WriteFile(backup, []byte("prior haproxy config"), 0o600); err != nil {
			t.Fatal(err)
		}
		return cfgPath
	}

	assertRolledBack := func(t *testing.T, err error, cfgPath string, bootstraps int) {
		t.Helper()
		if err == nil {
			t.Fatal("expected error when haproxy removal fails")
		}
		var ce *errtypes.ClusterError
		if !errors.As(err, &ce) {
			t.Fatalf("err is %T; want *errtypes.ClusterError", err)
		}
		if !strings.Contains(err.Error(), "haproxy removal failed") {
			t.Errorf("err = %q; want haproxy removal failure named", err.Error())
		}
		if bootstraps != 1 {
			t.Errorf("bootstrap dns rollback ran %d times; want 1", bootstraps)
		}
		got, readErr := os.ReadFile(cfgPath)
		if readErr != nil {
			t.Fatalf("haproxy config not restored: %v", readErr)
		}
		if string(got) != "prior haproxy config" {
			t.Errorf("restored haproxy config = %q; want backup content", got)
		}
	}

	// WorkDir has no cluster-config/auth/kubeconfig, so RemoveHAProxy's pre-flight CA load fails.
	t.Run("dns and haproxy config restored", func(t *testing.T) {
		_, bootstraps := stubDNSDeploys(t, nil, nil)
		cfgPath := seedHAProxyBackup(t)

		p := newTestPhase(t)
		_, err := p.finalizeIngress(context.Background(), ingressTestConfig(),
			UpdateIngressOptions{RemoveHAProxy: true, WorkDir: t.TempDir()},
			nil, nil, "10.0.0.40", "10.0.0.5", 0, 0)
		assertRolledBack(t, err, cfgPath, *bootstraps)
	})

	t.Run("haproxy config restored even when dns rollback fails", func(t *testing.T) {
		_, bootstraps := stubDNSDeploys(t, nil, errors.New("dnsmasq unavailable"))
		cfgPath := seedHAProxyBackup(t)

		p := newTestPhase(t)
		_, err := p.finalizeIngress(context.Background(), ingressTestConfig(),
			UpdateIngressOptions{RemoveHAProxy: true, WorkDir: t.TempDir()},
			nil, nil, "10.0.0.40", "10.0.0.5", 0, 0)
		assertRolledBack(t, err, cfgPath, *bootstraps)
	})
}

func TestReconcileBootstrapDNSOnly(t *testing.T) {
	t.Run("deploys dns pointing apps at the bastion", func(t *testing.T) {
		appsIPs, _ := stubDNSDeploys(t, nil, nil)

		p := newTestPhase(t)
		result, err := p.reconcileBootstrapDNSOnly(context.Background(), ingressTestConfig(), "10.0.0.5")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.KubeVipIP != "10.0.0.5" || !result.DNSReconciled {
			t.Errorf("result = %+v; want KubeVipIP 10.0.0.5 and DNSReconciled", result)
		}
		if len(*appsIPs) != 1 || (*appsIPs)[0] != "10.0.0.1" {
			t.Errorf("deploy called with apps IPs %v; want bastion [10.0.0.1]", *appsIPs)
		}
	})

	t.Run("deploy failure propagates", func(t *testing.T) {
		stubDNSDeploys(t, errors.New("dnsmasq restart failed"), nil)

		p := newTestPhase(t)
		if _, err := p.reconcileBootstrapDNSOnly(context.Background(), ingressTestConfig(), "10.0.0.5"); err == nil {
			t.Fatal("expected error when dns deploy fails")
		}
	})
}

func TestUpdateIngress_ConvertsAndCollects(t *testing.T) {
	installFakeOCForIngress(t)
	writeICList(t, "OC_IC_LIST_FILE",
		`{"metadata":{"name":"default"},"spec":{"endpointPublishingStrategy":{"type":"HostNetwork"}},"status":{"domain":"apps.a.test"}}`)
	writeICList(t, "OC_IC_LIST_FILE2",
		`{"metadata":{"name":"default"},"spec":{"endpointPublishingStrategy":{"type":"LoadBalancerService"}},"status":{"domain":"apps.a.test"}}`)
	tempEnvFile(t, "OC_IC_CALL_FILE")
	tempEnvFile(t, "OC_CALL_FILE")
	t.Setenv("OC_METALLB_NS", "1")
	t.Setenv("OC_METALLB_POOL", "1")
	t.Setenv("OC_SVC_IP_DEFAULT", "10.0.0.40")

	appsIPs, _ := stubDNSDeploys(t, nil, nil)

	p := newTestPhase(t)
	result, err := p.UpdateIngress(context.Background(), ingressTestConfig(),
		UpdateIngressOptions{ConfirmConversion: func([]string) bool { return true }})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ConvertedCount != 1 {
		t.Errorf("ConvertedCount = %d; want 1", result.ConvertedCount)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("entries = %+v; want exactly one", result.Entries)
	}
	entry := result.Entries[0]
	if entry.Name != "default" || entry.LBIP != "10.0.0.40" || !entry.Converted || entry.HostNetwork {
		t.Errorf("entry = %+v; want converted default with LBIP 10.0.0.40", entry)
	}
	if result.KubeVipIP != "10.0.0.5" || result.HAProxyRemoved || result.DNSReconciled {
		t.Errorf("result = %+v; want KubeVipIP 10.0.0.5 and no haproxy/dns-reconcile flags", result)
	}
	if len(*appsIPs) != 1 || (*appsIPs)[0] != "10.0.0.40" {
		t.Errorf("production dns deployed with apps IPs %v; want the collected LB IP [10.0.0.40]", *appsIPs)
	}
}

// The rollback derives a detached bounded context, so it still runs after the
// ctx that killed create is cancelled.
func TestConvertToLoadBalancer_RollbackRunsAfterCtxCancel(t *testing.T) {
	installFakeOCForIngress(t)

	counter := tempEnvFile(t, "OC_CALL_FILE")
	t.Setenv("OC_CREATE_SLEEP", "1")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Cancel only once the first create has started, so earlier probes run under a live ctx.
	go func() {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if data, err := os.ReadFile(counter); err == nil && strings.TrimSpace(string(data)) == "1" {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		cancel()
	}()

	p := newTestPhase(t)
	ic := minimalIC("default")

	err := p.convertToLoadBalancer(ctx, ic, 5*time.Second, "")
	if err == nil {
		t.Fatal("expected error when create is killed by ctx cancel")
	}
	if strings.Contains(err.Error(), "rollback also failed") {
		t.Fatalf("rollback must succeed under the detached ctx; got: %v", err)
	}

	raw, readErr := os.ReadFile(counter)
	if readErr != nil {
		t.Fatalf("counter file not written: %v", readErr)
	}
	if strings.TrimSpace(string(raw)) != "2" {
		t.Errorf("oc create call count = %q; want 2 (cancelled replacement + detached rollback)", strings.TrimSpace(string(raw)))
	}
}

func TestConvertToLoadBalancer_BackupLifecycle(t *testing.T) {
	newBackupEnv := func(t *testing.T) (backupPath, seen string) {
		t.Helper()
		installFakeOCForIngress(t)
		backupPath = ingressBackupPath(t.TempDir(), "default")
		seen = tempEnvFile(t, "OC_BACKUP_SEEN")
		tempEnvFile(t, "OC_CALL_FILE")
		t.Setenv("OC_BACKUP_EXPECT", backupPath)
		return backupPath, seen
	}

	assertSeenAtDelete := func(t *testing.T, seen string) {
		t.Helper()
		data, err := os.ReadFile(seen)
		if err != nil {
			t.Fatalf("delete never probed for the backup: %v", err)
		}
		if strings.TrimSpace(string(data)) != "yes" {
			t.Error("backup file must exist on disk before the delete is issued")
		}
	}

	t.Run("success removes backup", func(t *testing.T) {
		backupPath, seen := newBackupEnv(t)
		p := newTestPhase(t)
		if err := p.convertToLoadBalancer(context.Background(), minimalIC("default"), 5*time.Second, backupPath); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertSeenAtDelete(t, seen)
		if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
			t.Errorf("backup must be removed after successful conversion, stat err = %v", err)
		}
	})

	t.Run("successful rollback removes backup", func(t *testing.T) {
		backupPath, seen := newBackupEnv(t)
		t.Setenv("OC_CREATE_FAIL", "1")
		p := newTestPhase(t)
		if err := p.convertToLoadBalancer(context.Background(), minimalIC("default"), 5*time.Second, backupPath); err == nil {
			t.Fatal("expected error on create failure")
		}
		assertSeenAtDelete(t, seen)
		if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
			t.Errorf("backup must be removed after successful rollback, stat err = %v", err)
		}
	})

	t.Run("failed rollback retains backup", func(t *testing.T) {
		backupPath, _ := newBackupEnv(t)
		t.Setenv("OC_CREATE_FAIL", "1")
		t.Setenv("OC_ROLLBACK_FAIL", "1")
		p := newTestPhase(t)
		if err := p.convertToLoadBalancer(context.Background(), minimalIC("default"), 5*time.Second, backupPath); err == nil {
			t.Fatal("expected error when create and rollback both fail")
		}
		data, err := os.ReadFile(backupPath)
		if err != nil {
			t.Fatalf("backup must survive a failed rollback: %v", err)
		}
		want, buildErr := buildRollbackJSON(minimalIC("default"))
		if buildErr != nil {
			t.Fatalf("buildRollbackJSON: %v", buildErr)
		}
		if strings.TrimSpace(string(data)) != strings.TrimSpace(want) {
			t.Errorf("backup content mismatch:\ngot:  %s\nwant: %s", data, want)
		}
	})
}

func TestRestoreOrphanedIngressBackups(t *testing.T) {
	seedBackup := func(t *testing.T, workDir, name string) string {
		t.Helper()
		path := ingressBackupPath(workDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir cluster-config: %v", err)
		}
		payload, err := buildRollbackJSON(minimalIC(name))
		if err != nil {
			t.Fatalf("buildRollbackJSON: %v", err)
		}
		if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
			t.Fatalf("seed backup: %v", err)
		}
		return path
	}

	t.Run("missing controller restored and backup removed", func(t *testing.T) {
		installFakeOCForIngress(t)
		workDir := filepath.Join(t.TempDir(), "okd-install")
		path := seedBackup(t, workDir, "default")
		argvLog := tempEnvFile(t, "OC_ARGV_LOG")

		p := newTestPhase(t)
		restored := p.restoreOrphanedIngressBackups(context.Background(), workDir, nil)
		if restored != 1 {
			t.Fatalf("restored = %d; want 1", restored)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("backup must be removed after restore, stat err = %v", err)
		}
		argv, err := os.ReadFile(argvLog)
		if err != nil {
			t.Fatalf("argv log not written: %v", err)
		}
		if !strings.Contains(string(argv), "create -f -") {
			t.Errorf("expected an oc create -f - restore call; argv log:\n%s", argv)
		}
	})

	t.Run("present controller drops stale backup without create", func(t *testing.T) {
		installFakeOCForIngress(t)
		workDir := filepath.Join(t.TempDir(), "okd-install")
		path := seedBackup(t, workDir, "default")
		argvLog := tempEnvFile(t, "OC_ARGV_LOG")

		p := newTestPhase(t)
		restored := p.restoreOrphanedIngressBackups(context.Background(), workDir,
			[]ingressControllerInfo{{Name: "default"}})
		if restored != 0 {
			t.Fatalf("restored = %d; want 0", restored)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("stale backup must be removed, stat err = %v", err)
		}
		if argv, err := os.ReadFile(argvLog); err == nil && strings.Contains(string(argv), "create") {
			t.Errorf("no oc create expected for a stale backup; argv log:\n%s", argv)
		}
	})

	t.Run("restore failure keeps backup", func(t *testing.T) {
		installFakeOCForIngress(t)
		workDir := filepath.Join(t.TempDir(), "okd-install")
		path := seedBackup(t, workDir, "default")
		tempEnvFile(t, "OC_CALL_FILE")
		t.Setenv("OC_CREATE_FAIL", "1")

		p := newTestPhase(t)
		restored := p.restoreOrphanedIngressBackups(context.Background(), workDir, nil)
		if restored != 0 {
			t.Fatalf("restored = %d; want 0", restored)
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("backup must survive a failed restore: %v", err)
		}
	})
}
