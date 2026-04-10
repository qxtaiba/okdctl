//go:build linux || darwin

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
)

// TestCollectConfigSanitizedNoFile verifies the helper degrades gracefully
// when the named config file does not exist.
func TestCollectConfigSanitizedNoFile(t *testing.T) {
	out := collectConfigSanitized(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if !strings.Contains(out, "no config file") {
		t.Fatalf("expected 'no config file' message, got: %q", out)
	}
}

// TestCollectConfigSanitizedScrubsCredentials sets sentinel Password and
// APIToken values on a *config.Config, saves it via the loader, and runs
// the collector against the resulting file. The saved YAML relies on
// yaml:"-" tags to drop the secrets at marshal time — this test guards
// against regression in those tags by asserting the on-disk fixture is
// clean before the collector runs, and that the collector output stays
// clean end-to-end.
func TestCollectConfigSanitizedScrubsCredentials(t *testing.T) {
	const (
		sentinelPassword = "hunter2-password-sentinel"
		sentinelToken    = "test-api-token-sentinel"
		clusterName      = "diag-test-cluster"
	)

	cfg := config.DefaultConfig()
	cfg.Cluster.Name = clusterName
	if cfg.Provider.Proxmox == nil {
		t.Fatal("DefaultConfig did not initialise Provider.Proxmox")
	}
	cfg.Provider.Proxmox.Host = "192.168.1.100"
	cfg.Provider.Proxmox.Username = "root@pam"
	cfg.Provider.Proxmox.Password = sentinelPassword
	cfg.Provider.Proxmox.APIToken = sentinelToken

	path := filepath.Join(t.TempDir(), "openshitctl.yaml")
	loader := config.NewLoader()
	if err := loader.Save(cfg, path); err != nil {
		t.Fatalf("loader.Save: %v", err)
	}

	// As a sanity check on our fixture: confirm the credentials are NOT in
	// the on-disk YAML (they should be dropped by yaml:"-" tags). If this
	// fails, the test fixture itself is leaking, not the diag collector.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if strings.Contains(string(raw), sentinelPassword) || strings.Contains(string(raw), sentinelToken) {
		t.Fatalf("fixture YAML leaked credentials — yaml:\"-\" tags missing on Provider.Proxmox secret fields")
	}

	out := collectConfigSanitized(path)
	if strings.Contains(out, sentinelPassword) {
		t.Errorf("collectConfigSanitized leaked password: %q", out)
	}
	if strings.Contains(out, sentinelToken) {
		t.Errorf("collectConfigSanitized leaked API token: %q", out)
	}
	if !strings.Contains(out, clusterName) {
		t.Errorf("collectConfigSanitized dropped non-secret cluster.name; got: %q", out)
	}
}

// TestCollectConfigSanitizedRawPasswordLine writes a hand-crafted YAML
// fixture containing a plaintext password field (bypassing loader.Save's
// yaml:"-" filtering) and verifies the collector does not emit that
// value in its output. If viper/mapstructure populates Provider.Proxmox
// .Password from the raw YAML despite the yaml:"-" tag, this test
// directly exercises the in-memory zeroing block in collectConfigSanitized.
// If yaml:"-" is symmetric and viper ignores the field on load, the test
// still passes — but the mere fact that it passes documents which
// code path is load-bearing.
func TestCollectConfigSanitizedRawPasswordLine(t *testing.T) {
	const sentinel = "raw-yaml-password-sentinel"

	raw := `---
cluster:
  name: raw-fixture
  domain: k8s.local
distribution:
  type: okd
  version: "4.15.0"
provider:
  type: proxmox
  proxmox:
    host: 192.168.1.100
    node: pve
    username: root@pam
    password: ` + sentinel + `
topology:
  control_plane:
    count: 1
  workers:
    count: 0
networking:
  machine_cidr: 192.168.1.0/24
  gateway: 192.168.1.1
`

	path := filepath.Join(t.TempDir(), "openshitctl.yaml")
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	out := collectConfigSanitized(path)
	if strings.Contains(out, sentinel) {
		t.Errorf("collectConfigSanitized leaked raw-YAML password: %q", out)
	}
	// Non-secret fields from the fixture should still be visible — this
	// guards against the collector returning empty output on any error.
	if !strings.Contains(out, "raw-fixture") {
		t.Errorf("collectConfigSanitized dropped non-secret cluster.name; got: %q", out)
	}
}

// TestIsSensitiveKey checks the sensitive-key heuristic against the kinds
// of env var names openshitctl will encounter, both real (PROXMOX_VE_*) and
// idiomatic third-party patterns (ENVCHAIN-style). False positives matter:
// we do not want to mask PATH, HOME, or AUTHOR-style names.
func TestIsSensitiveKey(t *testing.T) {
	masked := []string{
		"PROXMOX_VE_PASSWORD",
		"PROXMOX_VE_API_TOKEN",
		"GITHUB_TOKEN",
		"AWS_SECRET_ACCESS_KEY",
		"DATABASE_PASSWORD",
		"OPENAI_API_KEY",
		"DOCKER_PASSWD",
		"VAULT_CREDENTIAL",
		"SSH_PRIVATE_KEY",
		"AWS_ACCESS_KEY_ID",
	}
	visible := []string{
		"PROXMOX_VE_USERNAME",
		"PROXMOX_VE_ENDPOINT",
		"PATH",
		"HOME",
		"USER",
		"GOPATH",
		"AUTHOR",
		"LANG",
		"TERM",
	}

	for _, k := range masked {
		if !isSensitiveKey(k) {
			t.Errorf("expected %q to be flagged as sensitive", k)
		}
	}
	for _, k := range visible {
		if isSensitiveKey(k) {
			t.Errorf("expected %q to be visible (false positive)", k)
		}
	}
}
