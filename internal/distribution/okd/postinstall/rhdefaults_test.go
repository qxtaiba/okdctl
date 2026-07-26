package postinstall

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/testutil"
)

func TestBuildOperatorHubPatch_DisablesOnlyNamedSources(t *testing.T) {
	out, err := buildOperatorHubPatch(disabledCatalogSources)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed struct {
		Spec struct {
			Sources []struct {
				Name     string `json:"name"`
				Disabled bool   `json:"disabled"`
			} `json:"sources"`
		} `json:"spec"`
	}
	if jsonErr := json.Unmarshal([]byte(out), &parsed); jsonErr != nil {
		t.Fatalf("patch is not valid JSON: %v\n%s", jsonErr, out)
	}

	if len(parsed.Spec.Sources) != 3 {
		t.Fatalf("sources = %d; want 3", len(parsed.Spec.Sources))
	}
	got := make(map[string]bool, len(parsed.Spec.Sources))
	for _, s := range parsed.Spec.Sources {
		got[s.Name] = s.Disabled
	}
	for _, name := range []string{"redhat-operators", "certified-operators", "redhat-marketplace"} {
		if !got[name] {
			t.Errorf("source %q missing or not disabled: %+v", name, parsed.Spec.Sources)
		}
	}
	if _, ok := got["community-operators"]; ok {
		t.Error("community-operators must not appear in the patch (left at its default enabled state)")
	}
}

// installFakeOCForRHDefaults installs a POSIX sh "oc" script logging argv to
// OC_ARGV_LOG and stdin (for `apply -f -`) to OC_STDIN_LOG, exiting
// OC_EXIT_CODE (default 0).
func installFakeOCForRHDefaults(t *testing.T) (argvLog, stdinLog string) {
	t.Helper()
	testutil.InstallFakeBin(t, "oc", `#!/bin/sh
echo "$@" >> "$OC_ARGV_LOG"
if [ "$1" = "apply" ]; then cat > "$OC_STDIN_LOG"; fi
exit "${OC_EXIT_CODE:-0}"
`)
	dir := t.TempDir()
	argvLog = filepath.Join(dir, "argv.log")
	stdinLog = filepath.Join(dir, "stdin.log")
	t.Setenv("OC_ARGV_LOG", argvLog)
	t.Setenv("OC_STDIN_LOG", stdinLog)
	return argvLog, stdinLog
}

func newRHDefaultsTestPhase(t *testing.T) *Phase {
	t.Helper()
	return New(phase.WithExecutor(executor.New()), phase.WithLogger(logutil.NopLogger))
}

func TestDisableSubscriptionGatedCatalogSources_PatchArgv(t *testing.T) {
	argvLog, _ := installFakeOCForRHDefaults(t)
	p := newRHDefaultsTestPhase(t)

	if err := p.disableSubscriptionGatedCatalogSources(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(argvLog)
	if err != nil {
		t.Fatalf("argv log not written: %v", err)
	}
	argv := strings.TrimSpace(string(data))
	if !strings.HasPrefix(argv, "patch operatorhub.config.openshift.io cluster --type=merge -p ") {
		t.Errorf("argv = %q; want it to target operatorhub.config.openshift.io/cluster with a merge patch", argv)
	}

	wantPatch, err := buildOperatorHubPatch(disabledCatalogSources)
	if err != nil {
		t.Fatalf("buildOperatorHubPatch: %v", err)
	}
	if !strings.HasSuffix(argv, wantPatch) {
		t.Errorf("argv payload = %q; want suffix %q", argv, wantPatch)
	}
}

func TestSilenceInsightsDisabledAlert_AppliesConfigMap(t *testing.T) {
	argvLog, stdinLog := installFakeOCForRHDefaults(t)
	p := newRHDefaultsTestPhase(t)

	if err := p.silenceInsightsDisabledAlert(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	argv, err := os.ReadFile(argvLog)
	if err != nil {
		t.Fatalf("argv log not written: %v", err)
	}
	if strings.TrimSpace(string(argv)) != "apply -f -" {
		t.Errorf("argv = %q; want %q", strings.TrimSpace(string(argv)), "apply -f -")
	}

	stdin, err := os.ReadFile(stdinLog)
	if err != nil {
		t.Fatalf("stdin log not written: %v", err)
	}
	for _, want := range []string{
		"kind: ConfigMap",
		"name: insights-config",
		"namespace: openshift-insights",
		"alerting:",
		"disabled: true",
	} {
		if !strings.Contains(string(stdin), want) {
			t.Errorf("applied manifest missing %q:\n%s", want, string(stdin))
		}
	}
}

func TestDisableRHDefaults_OcFailurePropagates(t *testing.T) {
	installFakeOCForRHDefaults(t)
	t.Setenv("OC_EXIT_CODE", "1")
	p := newRHDefaultsTestPhase(t)

	if err := p.disableSubscriptionGatedCatalogSources(context.Background()); err == nil {
		t.Fatal("expected error when oc patch exits non-zero")
	}
	if err := p.silenceInsightsDisabledAlert(context.Background()); err == nil {
		t.Fatal("expected error when oc apply exits non-zero")
	}
}
