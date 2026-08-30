package postinstall

import (
	"context"
	"encoding/json"
	"testing"

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

// installFakeOCForRHDefaults installs an oc that exits OC_EXIT_CODE.
func installFakeOCForRHDefaults(t *testing.T) {
	t.Helper()
	testutil.InstallFakeBin(t, "oc", `#!/bin/sh
exit "${OC_EXIT_CODE:-0}"
`)
}

func TestDisableRHDefaults_OcFailurePropagates(t *testing.T) {
	installFakeOCForRHDefaults(t)
	t.Setenv("OC_EXIT_CODE", "1")
	p := newTestPhase(t)

	if err := p.disableSubscriptionGatedCatalogSources(context.Background()); err == nil {
		t.Fatal("expected error when oc patch exits non-zero")
	}
	if err := p.silenceInsightsDisabledAlert(context.Background()); err == nil {
		t.Fatal("expected error when oc apply exits non-zero")
	}
}
