package steps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui"
)

func writeSecret(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(`{"auths":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReviewPreflightChecksFilesAndRunsValidate(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Files.PullSecret = writeSecret(t, "pull-secret.json")
	cfg.Files.SSHPublicKey = filepath.Join(t.TempDir(), "missing.pub")
	cfg.Networking.APIVIP = "10.0.0.5"

	got := reviewPreflight(cfg)
	if len(got) != 3 {
		t.Fatalf("reviewPreflight() returned %d checks, want 3: %#v", len(got), got)
	}
	if got[0] != (reviewCheck{label: "pull secret", status: "readable", detail: cfg.Files.PullSecret, passed: true}) {
		t.Errorf("pull secret check = %#v", got[0])
	}
	if got[1] != (reviewCheck{label: "ssh public key", status: statusUnavailable, detail: cfg.Files.SSHPublicKey, failed: true}) {
		t.Errorf("ssh key check = %#v", got[1])
	}
	if !got[2].failed || got[2].status != "1 error(s)" || !strings.Contains(got[2].detail, "networking.api_vip") {
		t.Errorf("configuration check = %#v, want one failing api_vip error", got[2])
	}
}

func TestReviewPreflightValidConfigPasses(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Files.PullSecret = writeSecret(t, "pull-secret.json")
	cfg.Files.SSHPublicKey = writeSecret(t, "id.pub")

	checks := reviewPreflight(cfg)
	for _, check := range checks {
		if !check.passed {
			t.Errorf("%s = %+v, want passed", check.label, check)
		}
	}
	if rendered := renderReviewPreflight(checks, 80); !strings.Contains(rendered, "3 passed · 0 warnings · 0 failed · 0 not checked") {
		t.Errorf("rendered header = %q", rendered)
	}
}

func TestReviewPreflightMissingRequiredFileIsBlockingNotUnchecked(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Files.PullSecret = filepath.Join(t.TempDir(), "missing.json")
	cfg.Files.SSHPublicKey = filepath.Join(t.TempDir(), "missing.pub")
	checks := reviewPreflight(cfg)

	for _, label := range []string{"pull secret", "ssh public key"} {
		var found bool
		for _, check := range checks {
			if check.label != label {
				continue
			}
			found = true
			if !check.failed || check.passed || check.warning {
				t.Errorf("%s = %+v, want a blocking failure", label, check)
			}
		}
		if !found {
			t.Fatalf("no %q check in %#v", label, checks)
		}
	}

	rendered := renderReviewPreflight(checks, 80)
	if !strings.Contains(rendered, tui.IconError) || !strings.Contains(rendered, "2 failed") {
		t.Errorf("rendered preflight lacks the error glyph or the failed count:\n%s", rendered)
	}
}

func TestReviewPreflightNonProxmoxCapacityIsNotApplicableNotUnchecked(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Provider.Type = "aws"
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
	if rendered := renderReviewPreflight(checks, 80); !strings.Contains(rendered, "0 not checked") {
		t.Errorf("not-applicable check inflated the not-checked count:\n%s", rendered)
	}
}

func TestReviewPreflightCapacityWithoutDiscoveryIsUnchecked(t *testing.T) {
	checks := reviewPreflight(config.DefaultConfig(), &WizardCapacitySnapshot{})
	if last := checks[len(checks)-1]; last.label != labelSelectedCapacity || last.status != statusNotChecked {
		t.Fatalf("capacity check without discovery = %+v, want not checked", last)
	}
}

func TestReviewPreflightRejectsSymlinkedSecretFiles(t *testing.T) {
	secret := writeSecret(t, "secret")
	link := filepath.Join(t.TempDir(), "pull-secret")
	if err := os.Symlink(secret, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if got := readableFileStatus(link); got != statusUnavailable {
		t.Fatalf("readableFileStatus(symlink) = %q, want unavailable", got)
	}
}
