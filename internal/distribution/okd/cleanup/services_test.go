package cleanup

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/dns"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

func redirectDnsmasqGlobs(t *testing.T, dir string) {
	t.Helper()
	origConf, origBackup := dnsmasqConfPattern, dnsmasqBackupPattern
	dnsmasqConfPattern = filepath.Join(dir, "okd-*.conf")
	dnsmasqBackupPattern = filepath.Join(dir, "*.backup")
	t.Cleanup(func() {
		dnsmasqConfPattern = origConf
		dnsmasqBackupPattern = origBackup
	})
}

// Guards the backup-residue fix: sweep both pristine and timestamped backups, not just timestamped.
func TestHAProxy_RemovesFixedAndTimestampedBackups(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "haproxy.cfg")
	pristine := cfg + phase.HAProxyBackupSuffix
	timestamped := phase.HAProxyTimestampedBackupPath(cfg, time.Now())

	for _, p := range []string{cfg, pristine, timestamped} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if err := HAProxy(context.Background(), cfg, "", logutil.NopLogger); err != nil {
		t.Fatalf("HAProxy: %v", err)
	}

	for _, p := range []string{cfg, pristine, timestamped} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s not removed by cleanup: %v", filepath.Base(p), err)
		}
	}
}

func TestDnsmasq_GlobLoopRemovesAllMatches(t *testing.T) {
	dir := t.TempDir()

	confs := []string{"okd-alpha.conf", "okd-beta.conf", "okd-gamma.conf"}
	for _, name := range confs {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	backups := []string{"resolv.backup", "dnsmasq.backup"}
	for _, name := range backups {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	redirectDnsmasqGlobs(t, dir)

	if err := Dnsmasq(context.Background(), "", logutil.NopLogger); err != nil {
		t.Fatalf("Dnsmasq: %v", err)
	}

	for _, name := range append(confs, backups...) {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s not removed after Dnsmasq run: %v", name, err)
		}
	}
}

func TestDnsmasq_RejectsClusterNameTraversalAtRegex(t *testing.T) {
	// Traversal is blocked upstream by validConfigNameRegex; the old
	// sentinel-in-tempdir test was vacuous.
	if _, err := dns.DnsmasqConfigPath("okd-../../../../etc/okd-x"); err == nil {
		t.Fatal("DnsmasqConfigPath accepted traversal-shaped name; want error from validConfigNameRegex")
	}

	// Dnsmasq() logs the rejection as a warning; it must not return an error.
	redirectDnsmasqGlobs(t, t.TempDir())

	if err := Dnsmasq(context.Background(), "../../../../etc/okd-x", logutil.NopLogger); err != nil {
		t.Fatalf("Dnsmasq returned unexpected error for traversal cluster name: %v", err)
	}
}
