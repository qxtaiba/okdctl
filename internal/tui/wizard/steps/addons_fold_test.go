package steps

import (
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

// TestAddonsFieldsFoldBehindEnableToggles pins the folded default: a
// disabled addon shows only its enable toggle, and enabling one unfolds its
// own settings without unfolding the other's.
func TestAddonsFieldsFoldBehindEnableToggles(t *testing.T) {
	s := NewAddonsStep()
	folded := tuitest.StripANSI(s.View(100, 80))
	for _, hidden := range []string{"repository", "provider", "secrets directory", "connect host"} {
		if strings.Contains(folded, hidden) {
			t.Errorf("folded addons view leaks %q:\n%s", hidden, folded)
		}
	}
	for _, shown := range []string{"gitops (flux)", "secret store", "enabled"} {
		if !strings.Contains(folded, shown) {
			t.Errorf("folded addons view missing %q:\n%s", shown, folded)
		}
	}

	cfg := config.DefaultConfig()
	cfg.Addons = map[string]config.AddonConfig{"flux": {Enabled: true}}
	unfolded := NewAddonsStep()
	unfolded.LoadFromConfig(cfg, true)
	view := tuitest.StripANSI(unfolded.View(100, 80))
	if !strings.Contains(view, "repository") {
		t.Errorf("enabling flux must unfold its settings:\n%s", view)
	}
	if strings.Contains(view, "connect host") {
		t.Errorf("enabling flux must not unfold the secret store sections:\n%s", view)
	}
}

// TestAddonsRequiredFieldsGateOnEnable pins Required-when-enabled: the flux
// repository and the bitwarden ids block validation only while their addon
// (and provider) is live.
func TestAddonsRequiredFieldsGateOnEnable(t *testing.T) {
	disabled := NewAddonsStep()
	if err := disabled.Validate(); err != nil {
		t.Fatalf("Validate() with everything disabled = %v, want nil", err)
	}

	cfg := config.DefaultConfig()
	cfg.Addons = map[string]config.AddonConfig{"flux": {Enabled: true}}
	flux := NewAddonsStep()
	flux.LoadFromConfig(cfg, true)
	if err := flux.Validate(); err == nil {
		t.Fatal("Validate() with flux enabled and no repository = nil, want the required error")
	}

	cfg = config.DefaultConfig()
	cfg.Addons = map[string]config.AddonConfig{"secretstore": {
		Enabled:  true,
		Settings: map[string]string{"provider": "bitwarden"},
	}}
	bw := NewAddonsStep()
	bw.LoadFromConfig(cfg, true)
	if err := bw.Validate(); err == nil {
		t.Fatal("Validate() with bitwarden enabled and no org id = nil, want the required error")
	}
}
