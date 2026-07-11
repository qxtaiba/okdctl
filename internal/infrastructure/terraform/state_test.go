package terraform

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

func TestSnapshotState_WritesBackup(t *testing.T) {
	dir := t.TempDir()
	payload := []byte(`{"version":4,"resources":[{"type":"proxmox_vm_qemu"}]}`)
	if err := os.WriteFile(filepath.Join(dir, "terraform.tfstate"), payload, 0o600); err != nil {
		t.Fatal(err)
	}

	e := &Executor{workDir: dir, logger: slog.New(&captureHandler{})}
	dst, err := e.SnapshotState(context.Background())
	if err != nil {
		t.Fatalf("SnapshotState: unexpected error: %v", err)
	}
	if dst == "" {
		t.Fatal("SnapshotState: returned empty path; expected backup path")
	}
	if !strings.HasPrefix(filepath.Base(dst), "terraform.tfstate.") || !strings.HasSuffix(dst, ".bak") {
		t.Errorf("backup name %q does not match terraform.tfstate.<ts>.bak pattern", filepath.Base(dst))
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("backup content mismatch\n got: %q\nwant: %q", got, payload)
	}

	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("stat backup: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("backup perm = %o; want 600", perm)
	}
}

func TestSnapshotState_AbsentSource(t *testing.T) {
	dir := t.TempDir()
	e := &Executor{workDir: dir, logger: slog.New(&captureHandler{})}

	dst, err := e.SnapshotState(context.Background())
	if err != nil {
		t.Fatalf("SnapshotState on missing source: %v", err)
	}
	if dst != "" {
		t.Errorf("expected empty path for absent source; got %q", dst)
	}
}

func TestSnapshotState_AtomicWriteError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses dir-mode write restrictions")
	}
	dir := t.TempDir()
	payload := []byte(`{"version":4,"resources":[]}`)
	if err := os.WriteFile(filepath.Join(dir, "terraform.tfstate"), payload, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	e := &Executor{workDir: dir, logger: slog.New(&captureHandler{})}
	dst, err := e.SnapshotState(context.Background())
	if err == nil {
		t.Fatalf("expected error from AtomicWrite failure; got nil (dst=%q)", dst)
	}
	if dst != "" {
		t.Errorf("expected empty path on write failure; got %q", dst)
	}
}

func TestPruneSnapshots_KeepsFiveMostRecent(t *testing.T) {
	dir := t.TempDir()

	names := []string{
		"terraform.tfstate.2024-01-01T00-00-00Z.bak",
		"terraform.tfstate.2024-01-02T00-00-00Z.bak",
		"terraform.tfstate.2024-01-03T00-00-00Z.bak",
		"terraform.tfstate.2024-01-04T00-00-00Z.bak",
		"terraform.tfstate.2024-01-05T00-00-00Z.bak",
		"terraform.tfstate.2024-01-06T00-00-00Z.bak",
		"terraform.tfstate.2024-01-07T00-00-00Z.bak",
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	e := &Executor{workDir: dir, logger: slog.New(&captureHandler{})}
	e.pruneSnapshots()

	removed := names[:2]
	kept := names[2:]

	for _, n := range removed {
		p := filepath.Join(dir, n)
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed; stat err: %v", n, err)
		}
	}
	for _, n := range kept {
		p := filepath.Join(dir, n)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s to be retained; stat err: %v", n, err)
		}
	}
}

func TestCheckStateMajorVersion_RejectsMajorTwo(t *testing.T) {
	dir := t.TempDir()
	sf := filepath.Join(dir, "terraform.tfstate")
	body := `{"version":4,"terraform_version":"2.0.0","resources":[]}`
	if err := os.WriteFile(sf, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	log := slog.New(&captureHandler{})
	err := checkStateMajorVersion(sf, log)
	if err == nil {
		t.Fatal("expected ConfigError for major=2; got nil")
	}
	var ce *errtypes.ConfigError
	if !errors.As(err, &ce) {
		t.Errorf("expected *errtypes.ConfigError; got %T: %v", err, err)
	}
}

func TestCheckStateMajorVersion_UnparseableVersionIsNonFatal(t *testing.T) {
	dir := t.TempDir()
	sf := filepath.Join(dir, "terraform.tfstate")
	body := `{"version":4,"terraform_version":"notasemver","resources":[]}`
	if err := os.WriteFile(sf, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	log := slog.New(&captureHandler{})
	if err := checkStateMajorVersion(sf, log); err != nil {
		t.Errorf("expected nil for unparseable version; got %v", err)
	}
}
