package steps

import (
	"os/exec"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

// TestAddonsStep_EnabledAddonsRequireTheirEndpoints pins bug 11's vault
// half: vault with an empty server url must fail wizard validation instead
// of an addon install an hour later — the flux repository moved to a
// field-level Required, covered by TestAddonsRequiredFieldsGateOnEnable.
func TestAddonsStep_EnabledAddonsRequireTheirEndpoints(t *testing.T) {
	if AddonsStepDefinition.Validate == nil {
		t.Fatal("AddonsStepDefinition.Validate is nil")
	}
	values := map[string]string{
		"secretstore_enabled":      valYes,
		"secretstore_provider":     providerVault,
		"secretstore_vault_server": "",
	}
	err := AddonsStepDefinition.Validate(values)
	if err == nil || !strings.Contains(err.Error(), "server url") {
		t.Fatalf("Validate(%v) = %v, want error naming the server url", values, err)
	}

	disabled := map[string]string{"flux_enabled": valNo, "secretstore_enabled": valNo}
	if err := AddonsStepDefinition.Validate(disabled); err != nil {
		t.Fatalf("Validate with everything disabled = %v, want nil", err)
	}
}

// TestAddonsStep_CrossFieldErrorFocusesVaultServerField pins the
// reconciliation-audit defect's addons half: the vault-server cross-field
// failure only ever reached the status-row banner, never the vault server
// field itself, so an operator focused elsewhere saw a complaint with no
// indication of where to fix it.
func TestAddonsStep_CrossFieldErrorFocusesVaultServerField(t *testing.T) {
	step := NewAddonsStep()
	step.SetFocused(true)
	step.SetValue("secretstore_enabled", valYes)
	step.SetValue("secretstore_provider", providerVault)

	_, cmd := step.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Update(enter) with a missing vault server url: want a cmd, got nil")
	}
	if !containsFocusChanged(cmd) {
		t.Fatal("Update(enter) with a cross-field error did not emit FocusChangedMsg")
	}

	label, _, ok := step.FocusedFieldHelp()
	if !ok || label != "server url" {
		t.Fatalf("FocusedFieldHelp() label = %q, ok=%v, want the vault server field focused", label, ok)
	}

	view := tuitest.StripANSI(step.View(100, 30))
	if !strings.Contains(view, "vault server url is required") {
		t.Fatalf("View() after a cross-field error = %q, want the inline error visible", view)
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
				"secretstore": {Enabled: true, Settings: map[string]string{"provider": tc.provider}},
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

// TestAddonsStep_ViewDoesNotCallSopsLookup pins the reconciliation-audit
// defect where View synchronously ran exec.LookPath("sops") on every
// render whenever secretstore was enabled, violating the contract that
// View performs no file/PATH/network checks.
func TestAddonsStep_ViewDoesNotCallSopsLookup(t *testing.T) {
	calls := 0
	prev := sopsOnPath
	sopsOnPath = func() bool {
		calls++
		return true
	}
	defer func() { sopsOnPath = prev }()

	cfg := config.DefaultConfig()
	cfg.Addons = map[string]config.AddonConfig{"secretstore": {Enabled: true}}

	step := NewAddonsStep()
	step.LoadFromConfig(cfg, true)

	afterConstruct := calls

	for range 5 {
		step.View(100, 30)
	}

	if calls != afterConstruct {
		t.Fatalf("View() called the sops lookup %d extra time(s); it must resolve once outside the render path, not on every View()", calls-afterConstruct)
	}
}
