package steps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui"
)

func TestReviewPreflightChecksFilesAndCIDROverlap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pull-secret.json")
	if err := os.WriteFile(path, []byte(`{"auths":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	cfg.Files.PullSecret = path
	cfg.Files.SSHPublicKey = filepath.Join(t.TempDir(), "missing.pub")
	cfg.Networking.MachineCIDR = "10.0.0.0/16"
	cfg.Networking.PodCIDR = "10.0.1.0/24"
	cfg.Networking.ServiceCIDR = "172.30.0.0/16"

	got := reviewPreflight(cfg)
	want := []reviewCheck{
		{label: "pull secret", status: "readable", detail: path, passed: true},
		{label: "ssh public key", status: "unavailable", detail: cfg.Files.SSHPublicKey, failed: true},
		{label: "CIDR ranges", status: "overlap detected", detail: "10.0.0.0/16 · 10.0.1.0/24 · 172.30.0.0/16", warning: true},
	}
	if len(got) != len(want) {
		t.Fatalf("reviewPreflight() returned %d checks, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("check %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestReviewPreflightChecksEnabledAddonPrerequisites(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PATH", bin)

	sopsPath := filepath.Join(bin, "sops")
	if err := os.WriteFile(sopsPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfig()
	cfg.Addons["flux"] = config.AddonConfig{Enabled: true}
	cfg.Addons["secretstore"] = config.AddonConfig{Enabled: true}
	checks := reviewPreflight(cfg)
	byLabel := make(map[string]reviewCheck, len(checks))
	for _, check := range checks {
		byLabel[check.label] = check
	}
	if got := byLabel["flux deploy key"]; got.detail != "~/.ssh/flux-deploy-key" ||
		(got.status != "readable" && got.status != statusUnavailable) || got.passed != (got.status == "readable") {
		t.Errorf("flux key preflight = %+v, want an honest readable/unavailable result", got)
	}
	if got := byLabel["sops"]; got.status != "available" || !got.passed || got.warning {
		t.Errorf("sops preflight = %+v, want available", got)
	}

	cfg.Addons["flux"] = config.AddonConfig{}
	cfg.Addons["secretstore"] = config.AddonConfig{}
	for _, check := range reviewPreflight(cfg) {
		if check.label == "flux deploy key" || check.label == "sops" {
			t.Errorf("disabled addon prerequisite included in preflight: %+v", check)
		}
	}
}

func TestReviewPreflightLeavesAbsentChecksUnconfirmed(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Networking.MachineCIDR = ""
	cfg.Networking.PodCIDR = ""
	cfg.Networking.ServiceCIDR = ""
	got := reviewPreflight(cfg)
	if got[0].status != "not configured" || got[1].status != "not configured" || got[2].status != "not checked" {
		t.Fatalf("reviewPreflight() = %#v, want absent inputs called out without false passes", got)
	}
}

// TestReviewPreflightMissingRequiredFileIsBlockingNotUnchecked pins the
// honest-state fix: a required input that is missing or unreadable must
// read as a blocking failure with the error glyph and its own "failed"
// count, never the same pending state as a check that simply hasn't run.
func TestReviewPreflightMissingRequiredFileIsBlockingNotUnchecked(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Networking.MachineCIDR = ""
	cfg.Networking.PodCIDR = ""
	cfg.Networking.ServiceCIDR = ""
	checks := reviewPreflight(cfg)

	for _, label := range []string{"pull secret", "ssh public key"} {
		var found bool
		for _, check := range checks {
			if check.label != label {
				continue
			}
			found = true
			if !check.failed || check.passed || check.warning {
				t.Errorf("%s = %+v, want a blocking failure, not an unchecked/warning state", label, check)
			}
		}
		if !found {
			t.Fatalf("no %q check in %#v", label, checks)
		}
	}

	rendered := renderReviewPreflight(checks, 80)
	if !strings.Contains(rendered, tui.IconError) {
		t.Errorf("rendered preflight has no error glyph for the missing required files:\n%s", rendered)
	}
	if !strings.Contains(rendered, "2 failed") {
		t.Errorf("rendered preflight header omits its own failed count:\n%s", rendered)
	}
}

// TestReviewPreflightNonProxmoxCapacityIsNotApplicableNotUnchecked pins the
// other honest-state fix: a capacity check that doesn't apply to a
// non-Proxmox provider must never inflate the "not checked" count, which
// otherwise could never reach zero on such a config.
func TestReviewPreflightNonProxmoxCapacityIsNotApplicableNotUnchecked(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Files.PullSecret = "/nonexistent/pull-secret.json"
	cfg.Files.SSHPublicKey = "/nonexistent/id_ed25519.pub"
	cfg.Provider.Type = "aws"
	cfg.Provider.Proxmox = nil
	cfg.Networking.MachineCIDR = "10.0.0.0/16"
	cfg.Networking.PodCIDR = "10.0.1.0/24"
	cfg.Networking.ServiceCIDR = "172.30.0.0/16"
	checks := reviewPreflight(cfg, &WizardCapacitySnapshot{})

	var capacity reviewCheck
	for _, check := range checks {
		if check.label == labelSelectedCapacity {
			capacity = check
		}
	}
	if !capacity.notApplicable {
		t.Fatalf("selected capacity check = %+v, want notApplicable", capacity)
	}

	rendered := renderReviewPreflight(checks, 80)
	if !strings.Contains(rendered, "0 not checked") {
		t.Errorf("not-applicable check inflated the not-checked count above zero:\n%s", rendered)
	}
}

func TestReviewPreflightRejectsSymlinkedSecretFiles(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "secret")
	link := filepath.Join(t.TempDir(), "pull-secret")
	if err := os.WriteFile(secret, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if got := readableFileStatus(link); got != "unavailable" {
		t.Fatalf("readableFileStatus(symlink) = %q, want unavailable", got)
	}
}
