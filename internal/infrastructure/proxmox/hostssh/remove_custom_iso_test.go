package hostssh

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveCustomISOsFromProxmox_removesUnreferencedISOs(t *testing.T) {
	installFakeSSH(t)
	dir := t.TempDir()
	counter := filepath.Join(dir, "rm-counter")
	t.Setenv("SSH_FAKE_MODE", "no-ref")
	t.Setenv("SSH_RM_COUNTER", counter)

	p := newTestISOParams(t)
	names := []string{"bootstrap.iso", "master0.iso"}
	if err := RemoveCustomISOsFromProxmox(context.Background(), p, "/var/lib/vz/template/iso", names); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	raw, readErr := os.ReadFile(counter)
	if readErr != nil {
		t.Fatalf("rm counter file not written — rm was not called: %v", readErr)
	}
	if string(raw) != "2" {
		t.Errorf("rm counter = %q; want 2 (one per name)", string(raw))
	}
}

func TestRemoveCustomISOsFromProxmox_skipsISOReferencedByRunningVM(t *testing.T) {
	installFakeSSH(t)
	dir := t.TempDir()
	counter := filepath.Join(dir, "rm-counter")
	t.Setenv("SSH_FAKE_MODE", "in-use")
	t.Setenv("SSH_RM_COUNTER", counter)

	p := newTestISOParams(t)
	// installFakeSSH's in-use fixture references fedora-coreos-40.20240101.iso
	// verbatim, so this name is the one that must match anyVMReferencesISO.
	names := []string{"fedora-coreos-40.20240101.iso"}
	if err := RemoveCustomISOsFromProxmox(context.Background(), p, "/var/lib/vz/template/iso", names); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, statErr := os.Stat(counter); statErr == nil {
		raw, _ := os.ReadFile(counter)
		t.Errorf("rm called (counter=%q) but iso is in use — removal must be skipped", string(raw))
	}
}

func TestRemoveCustomISOsFromProxmox_skipsUnsafeFilename(t *testing.T) {
	installFakeSSH(t)
	dir := t.TempDir()
	counter := filepath.Join(dir, "rm-counter")
	t.Setenv("SSH_FAKE_MODE", "no-ref")
	t.Setenv("SSH_RM_COUNTER", counter)

	p := newTestISOParams(t)
	names := []string{"../escape.iso", "sub/dir.iso"}
	if err := RemoveCustomISOsFromProxmox(context.Background(), p, "/var/lib/vz/template/iso", names); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, statErr := os.Stat(counter); statErr == nil {
		raw, _ := os.ReadFile(counter)
		t.Errorf("rm called (counter=%q) but every name is unsafe — removal must be skipped", string(raw))
	}
}

func TestRemoveCustomISOsFromProxmox_rejectsUnsafeISODir(t *testing.T) {
	p := newTestISOParams(t)
	err := RemoveCustomISOsFromProxmox(context.Background(), p, "relative/dir", []string{"bootstrap.iso"})
	if err == nil {
		t.Fatal("expected error for non-absolute isoDir; got nil")
	}
}
