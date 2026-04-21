package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOpenLogFileRefusesSymlink locks the sec:00000002 fix: --log-file
// must refuse a symlinked path so a pre-sudo attacker cannot redirect
// root-authored log lines onto an arbitrary file between the invoking-
// user run and the sudo re-exec.
func TestOpenLogFileRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.log")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	link := filepath.Join(dir, "link.log")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	f, err := openLogFile(link)
	if err == nil {
		_ = f.Close()
		t.Fatal("openLogFile accepted a symlink; sec:00000002 guard regressed")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("error does not name the rejection reason: %v", err)
	}
}

// TestOpenLogFileAcceptsRegularFile covers the positive path so the
// symlink refusal doesn't silently break normal --log-file usage.
func TestOpenLogFileAcceptsRegularFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ok.log")

	f, err := openLogFile(path)
	if err != nil {
		t.Fatalf("openLogFile on fresh path: %v", err)
	}
	_ = f.Close()

	f, err = openLogFile(path)
	if err != nil {
		t.Fatalf("openLogFile on existing regular file: %v", err)
	}
	_ = f.Close()
}
