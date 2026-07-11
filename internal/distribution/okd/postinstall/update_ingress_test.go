package postinstall

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

func TestBuildLBIngressController_PreservesFields(t *testing.T) {
	cases := []struct {
		name        string
		icJSON      json.RawMessage
		hasReplicas bool
		hasCert     bool
		hasRoute    bool
		hasAdmit    bool
		hasNode     bool
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
			hasReplicas: true,
			hasCert:     true,
			hasRoute:    true,
			hasAdmit:    true,
			hasNode:     true,
		},
		{
			name: "absent optional fields stay omitted",
			icJSON: json.RawMessage(`{
				"metadata":{"name":"minimal","namespace":"openshift-ingress-operator"},
				"spec":{"domain":"apps.minimal.test"}
			}`),
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
			if parsed.Metadata.Namespace == "" {
				t.Error("Namespace must not be empty")
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

func TestBuildLBIngressController_TypeIsLoadBalancerService(t *testing.T) {
	ic := &ingressControllerInfo{
		RawJSON: json.RawMessage(`{
			"metadata":{"name":"default","namespace":"openshift-ingress-operator"},
			"spec":{
				"domain":"apps.cluster.local",
				"endpointPublishingStrategy":{"type":"HostNetwork"}
			}
		}`),
	}
	out, err := buildLBIngressController(ic)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed struct {
		Spec struct {
			EndpointPublishingStrategy struct {
				Type string `json:"type"`
			} `json:"endpointPublishingStrategy"`
		} `json:"spec"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if parsed.Spec.EndpointPublishingStrategy.Type != "LoadBalancerService" {
		t.Errorf("Type = %q; want LoadBalancerService", parsed.Spec.EndpointPublishingStrategy.Type)
	}
}

func TestBuildLBIngressController_PreservesSpecFields(t *testing.T) {
	wantDomain := "apps.example.com"
	wantReplicas := int32(3)
	ic := &ingressControllerInfo{
		RawJSON: json.RawMessage(`{
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
	}

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
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	if parsed.Spec.EndpointPublishingStrategy.Type != "LoadBalancerService" {
		t.Errorf("strategy type = %q; want LoadBalancerService", parsed.Spec.EndpointPublishingStrategy.Type)
	}
	if parsed.Spec.Domain != wantDomain {
		t.Errorf("domain = %q; want %q", parsed.Spec.Domain, wantDomain)
	}
	if parsed.Spec.Replicas == nil || *parsed.Spec.Replicas != wantReplicas {
		t.Errorf("replicas = %v; want %d", parsed.Spec.Replicas, wantReplicas)
	}
	if parsed.Spec.DefaultCertificate == nil {
		t.Error("defaultCertificate must be present but is nil")
	}
	if parsed.Spec.RouteSelector == nil {
		t.Error("routeSelector must be present but is nil")
	}
	if parsed.Spec.RouteAdmission == nil {
		t.Error("routeAdmission must be present but is nil")
	}
	if parsed.Spec.NodePlacement == nil {
		t.Error("nodePlacement must be present but is nil")
	}
}

func TestBuildLBIngressController_EmptyNamespaceDefaults(t *testing.T) {
	ic := &ingressControllerInfo{
		RawJSON: json.RawMessage(`{
			"metadata":{"name":"default","namespace":""},
			"spec":{"domain":"apps.cluster.local"}
		}`),
	}

	out, err := buildLBIngressController(ic)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed struct {
		Metadata struct {
			Namespace string `json:"namespace"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if parsed.Metadata.Namespace != "openshift-ingress-operator" {
		t.Errorf("namespace = %q; want openshift-ingress-operator", parsed.Metadata.Namespace)
	}
}

// installFakeOCForIngress writes a POSIX sh script named "oc" into a temp dir
// and prepends it to PATH. The script dispatches on $1 with env-var overrides:
//   - OC_ARGV_LOG           → path; every call appends all args as one line
//   - OC_DELETE_FAIL=1      → delete exits 1
//   - OC_CALL_FILE          → path; incremented on each create invocation
//   - OC_CREATE_FAIL=1      → first create (n=1) exits 1
//   - OC_ROLLBACK_FAIL=1    → second create (n>=2) exits 1
//   - OC_ROLLBACK_STDIN_LOG → path; second create writes its stdin there
func installFakeOCForIngress(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-oc script relies on POSIX sh")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"if [ -n \"${OC_ARGV_LOG:-}\" ]; then echo \"$*\" >> \"$OC_ARGV_LOG\"; fi\n" +
		"case \"$1\" in\n" +
		"  delete)\n" +
		"    if [ \"${OC_DELETE_FAIL:-0}\" = \"1\" ]; then echo \"fake: delete failed\" >&2; exit 1; fi\n" +
		"    exit 0\n" +
		"    ;;\n" +
		"  get)\n" +
		"    exit 0\n" +
		"    ;;\n" +
		"  create)\n" +
		"    f=\"${OC_CALL_FILE:-/tmp/okd-ingress-counter}\"\n" +
		"    n=$(cat \"$f\" 2>/dev/null || echo 0)\n" +
		"    n=$((n + 1))\n" +
		"    echo \"$n\" > \"$f\"\n" +
		"    if [ \"$n\" -ge 2 ] && [ -n \"${OC_ROLLBACK_STDIN_LOG:-}\" ]; then cat > \"$OC_ROLLBACK_STDIN_LOG\"; fi\n" +
		"    if [ \"$n\" -eq 1 ] && [ \"${OC_CREATE_FAIL:-0}\" = \"1\" ]; then echo \"fake: create failed\" >&2; exit 1; fi\n" +
		"    if [ \"$n\" -ge 2 ] && [ \"${OC_ROLLBACK_FAIL:-0}\" = \"1\" ]; then echo \"fake: rollback failed\" >&2; exit 1; fi\n" +
		"    exit 0\n" +
		"    ;;\n" +
		"  *) exit 0 ;;\n" +
		"esac\n"
	path := filepath.Join(dir, "oc")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func newIngressTestPhase(t *testing.T) *Phase {
	t.Helper()
	return New(
		phase.WithExecutor(executor.New()),
		phase.WithLogger(logutil.NopLogger),
	)
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

// TestConvertToLoadBalancer_DeleteArgvTargetsICName asserts that the oc delete
// call names only ic.Name in namespace openshift-ingress-operator.
func TestConvertToLoadBalancer_DeleteArgvTargetsICName(t *testing.T) {
	installFakeOCForIngress(t)

	dir := t.TempDir()
	argvLog := filepath.Join(dir, "argv.log")
	counter := filepath.Join(dir, "counter")
	t.Setenv("OC_ARGV_LOG", argvLog)
	t.Setenv("OC_CALL_FILE", counter)

	p := newIngressTestPhase(t)
	ic := minimalIC("custom")

	if err := p.convertToLoadBalancer(context.Background(), ic, 5*time.Second); err != nil {
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

// TestConvertToLoadBalancer_CreateFailure_RollbackIssued asserts that a failed
// replacement create triggers attemptRollback, which issues a second oc create
// whose stdin matches buildRollbackJSON output.
func TestConvertToLoadBalancer_CreateFailure_RollbackIssued(t *testing.T) {
	installFakeOCForIngress(t)

	dir := t.TempDir()
	counter := filepath.Join(dir, "counter")
	rollbackStdin := filepath.Join(dir, "rollback-stdin.json")
	t.Setenv("OC_CALL_FILE", counter)
	t.Setenv("OC_CREATE_FAIL", "1")
	t.Setenv("OC_ROLLBACK_STDIN_LOG", rollbackStdin)

	p := newIngressTestPhase(t)
	ic := minimalIC("default")

	if err := p.convertToLoadBalancer(context.Background(), ic, 5*time.Second); err == nil {
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

// TestConvertToLoadBalancer_BothCreateAndRollbackFail_ErrorNamesBoth asserts that
// when both replacement create and rollback create fail, the returned error
// message names both failures.
func TestConvertToLoadBalancer_BothCreateAndRollbackFail_ErrorNamesBoth(t *testing.T) {
	installFakeOCForIngress(t)

	dir := t.TempDir()
	counter := filepath.Join(dir, "counter")
	t.Setenv("OC_CALL_FILE", counter)
	t.Setenv("OC_CREATE_FAIL", "1")
	t.Setenv("OC_ROLLBACK_FAIL", "1")

	p := newIngressTestPhase(t)
	ic := minimalIC("default")

	err := p.convertToLoadBalancer(context.Background(), ic, 5*time.Second)
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

// TestRestoreHAProxyBackup_PrefersTimestampedFallsBackToPristine covers the
// restore side of the shared backup contract: the newest timestamped backup
// wins when present, and setup's fixed pristine snapshot is the fallback
// when RemoveHAProxy left no timestamped backup.
func TestRestoreHAProxyBackup_PrefersTimestampedFallsBackToPristine(t *testing.T) {
	newPhase := func() *Phase {
		return New(phase.WithExecutor(executor.New()), phase.WithLogger(logutil.NopLogger))
	}
	setConfigPath := func(t *testing.T, cfg string) {
		t.Helper()
		orig := haproxyConfigPath
		t.Cleanup(func() { haproxyConfigPath = orig })
		haproxyConfigPath = cfg
	}

	t.Run("prefers newest timestamped backup", func(t *testing.T) {
		cfg := filepath.Join(t.TempDir(), "haproxy.cfg")
		setConfigPath(t, cfg)

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

		if !newPhase().restoreHAProxyBackup() {
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
		setConfigPath(t, cfg)

		if err := os.WriteFile(cfg+phase.HAProxyBackupSuffix, []byte("pristine"), 0o600); err != nil {
			t.Fatal(err)
		}

		if !newPhase().restoreHAProxyBackup() {
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
		setConfigPath(t, cfg)

		if newPhase().restoreHAProxyBackup() {
			t.Fatal("restoreHAProxyBackup returned true with no backup files")
		}
	})
}
