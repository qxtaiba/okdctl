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

func TestAddonsStep_WarningAttachesToFluxSection(t *testing.T) {
	if system.FileExists(system.ExpandPath("~/.ssh/flux-deploy-key")) {
		t.Skip("~/.ssh/flux-deploy-key exists on this machine, so the warning this test checks for would not fire")
	}

	cfg := config.DefaultConfig()
	cfg.Addons = map[string]config.AddonConfig{"flux": {Enabled: true}}

	step := NewAddonsStep()
	step.LoadFromConfig(cfg)

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
	step.LoadFromConfig(cfg)

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
