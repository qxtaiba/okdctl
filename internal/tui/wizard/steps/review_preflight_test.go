package steps

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
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
		{label: "pull secret", status: "readable"},
		{label: "ssh public key", status: "unavailable", warning: true},
		{label: "CIDR ranges", status: "overlap detected", warning: true},
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
