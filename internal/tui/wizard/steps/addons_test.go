package steps

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

// TestAddonsStep_EnabledAddonsRequireTheirEndpoints pins bug 11: flux with
// an empty repository or vault with an empty server url must fail wizard
// validation instead of an addon install an hour later.
func TestAddonsStep_EnabledAddonsRequireTheirEndpoints(t *testing.T) {
	cases := []struct {
		name   string
		values map[string]string
		want   string
	}{
		{
			name:   "flux enabled without repository",
			values: map[string]string{"flux_enabled": valYes, "flux_repository": ""},
			want:   "repository",
		},
		{
			name: "vault provider without server url",
			values: map[string]string{
				"secretstore_enabled":      valYes,
				"secretstore_provider":     providerVault,
				"secretstore_vault_server": "",
			},
			want: "server url",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if AddonsStepDefinition.Validate == nil {
				t.Fatal("AddonsStepDefinition.Validate is nil")
			}
			err := AddonsStepDefinition.Validate(tc.values)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate(%v) = %v, want error naming %q", tc.values, err, tc.want)
			}
		})
	}

	disabled := map[string]string{"flux_enabled": valNo, "secretstore_enabled": valNo}
	if err := AddonsStepDefinition.Validate(disabled); err != nil {
		t.Fatalf("Validate with everything disabled = %v, want nil", err)
	}
}

func TestAddonsStep_OnlyTheSelectedProviderSectionRenders(t *testing.T) {
	sectionTitles := []string{"secret store (onepassword)", "secret store (vault)", "secret store (bitwarden)"}

	cases := []struct {
		provider string
		want     string
	}{
		{provider: "onepassword", want: "secret store (onepassword)"},
		{provider: "vault", want: "secret store (vault)"},
		{provider: "bitwarden", want: "secret store (bitwarden)"},
	}

	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.Addons = map[string]config.AddonConfig{
				"secretstore": {Settings: map[string]string{"provider": tc.provider}},
			}

			step := NewAddonsStep()
			step.LoadFromConfig(cfg, true)

			view := tuitest.StripANSI(step.View(100, 30))
			for _, title := range sectionTitles {
				got := strings.Contains(view, title)
				want := title == tc.want
				if got != want {
					t.Fatalf("provider %q: section %q present=%v, want %v", tc.provider, title, got, want)
				}
			}
		})
	}
}

func TestAddonsStep_WarningAttachesToFluxSection(t *testing.T) {
	if system.FileExists(system.ExpandPath("~/.ssh/flux-deploy-key")) {
		t.Skip("~/.ssh/flux-deploy-key exists on this machine, so the warning this test checks for would not fire")
	}

	cfg := config.DefaultConfig()
	cfg.Addons = map[string]config.AddonConfig{"flux": {Enabled: true}}

	step := NewAddonsStep()
	step.LoadFromConfig(cfg, false)

	lines := strings.Split(tuitest.StripANSI(step.View(100, 30)), "\n")

	pathIdx, warnIdx, nextSectionIdx := -1, -1, -1
	for i, line := range lines {
		switch {
		case strings.Contains(line, "kubernetes/clusters/production"):
			pathIdx = i
		case warnIdx == -1 && strings.Contains(line, tui.IconWarning):
			warnIdx = i
		case nextSectionIdx == -1 && strings.Contains(line, "secret store (common)"):
			nextSectionIdx = i
		}
	}

	if pathIdx == -1 {
		t.Fatal("could not locate the flux_path field in the rendered view")
	}
	if warnIdx == -1 {
		t.Fatal("no warning rendered for an enabled flux section missing its ssh deploy key")
	}
	if nextSectionIdx == -1 {
		t.Fatal("could not locate the secret store (common) section head in the rendered view")
	}
	if pathIdx >= warnIdx || warnIdx >= nextSectionIdx {
		t.Fatalf("want flux_path (%d) < warning (%d) < secret store section (%d)", pathIdx, warnIdx, nextSectionIdx)
	}
}

func TestAddonsStep_WarningAttachesToSecretStoreSection(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := exec.LookPath("sops"); err == nil {
		t.Fatal("sops still resolves on the overridden PATH, so the warning this test checks for would not fire")
	}

	cfg := config.DefaultConfig()
	cfg.Addons = map[string]config.AddonConfig{"secretstore": {Enabled: true}}

	step := NewAddonsStep()
	step.LoadFromConfig(cfg, false)

	lines := strings.Split(tuitest.StripANSI(step.View(100, 30)), "\n")

	secretsDirIdx, warnIdx, nextSectionIdx := -1, -1, -1
	for i, line := range lines {
		switch {
		case strings.Contains(line, "automation/config/secrets"):
			secretsDirIdx = i
		case warnIdx == -1 && strings.Contains(line, tui.IconWarning):
			warnIdx = i
		case nextSectionIdx == -1 && strings.Contains(line, "secret store (onepassword)"):
			nextSectionIdx = i
		}
	}

	if secretsDirIdx == -1 {
		t.Fatal("could not locate the secretstore_secrets_dir field in the rendered view")
	}
	if warnIdx == -1 {
		t.Fatal("no warning rendered for an enabled secretstore section missing sops")
	}
	if nextSectionIdx == -1 {
		t.Fatal("could not locate the secret store (onepassword) section head in the rendered view")
	}
	if secretsDirIdx >= warnIdx || warnIdx >= nextSectionIdx {
		t.Fatalf("want secretstore_secrets_dir (%d) < warning (%d) < secret store (onepassword) section (%d)", secretsDirIdx, warnIdx, nextSectionIdx)
	}
}
