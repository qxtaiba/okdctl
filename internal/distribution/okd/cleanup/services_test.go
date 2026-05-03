package cleanup

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

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

	orig1 := dnsmasqConfPattern
	orig2 := dnsmasqBackupPattern
	dnsmasqConfPattern = filepath.Join(dir, "okd-*.conf")
	dnsmasqBackupPattern = filepath.Join(dir, "*.backup")
	t.Cleanup(func() {
		dnsmasqConfPattern = orig1
		dnsmasqBackupPattern = orig2
	})

	if err := Dnsmasq(context.Background(), "", logutil.NopLogger); err != nil {
		t.Fatalf("Dnsmasq: %v", err)
	}

	for _, name := range append(confs, backups...) {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s not removed after Dnsmasq run: %v", name, err)
		}
	}
}
