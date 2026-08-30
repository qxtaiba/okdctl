package phase

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestHAProxyBackupGlob_CoversPristineAndTimestamped(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "haproxy.cfg")
	pristine := cfg + HAProxyBackupSuffix
	older := HAProxyTimestampedBackupPath(cfg, time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC))
	newer := HAProxyTimestampedBackupPath(cfg, time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC))

	for _, p := range []string{cfg, pristine, older, newer} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	matches, err := filepath.Glob(HAProxyBackupGlob(cfg))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}

	want := []string{pristine, older, newer}
	slices.Sort(matches)
	slices.Sort(want)
	if !slices.Equal(matches, want) {
		t.Fatalf("glob matches = %v, want %v (live config must not match; pristine and timestamped must)", matches, want)
	}

	if got := slices.Max(matches); got != newer {
		t.Errorf("slices.Max = %q, want newest timestamped backup %q", got, newer)
	}
}
