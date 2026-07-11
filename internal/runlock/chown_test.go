package runlock

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// Both tests verify the euid gate: as non-root, SUDO_UID/SUDO_GID pointing
// at another user must be ignored — without the gate the chown would fail
// EPERM and (in Acquire) emit a spurious warning. The chown-as-root branch
// needs euid 0 and is intentionally untested here.

func TestChownToSudoInvoker_NonRootIgnoresSudoEnv(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("euid gate is only observable as non-root")
	}
	t.Setenv("SUDO_UID", "0")
	t.Setenv("SUDO_GID", "0")

	path := filepath.Join(t.TempDir(), ".okdctl.lock")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("write lockfile: %v", err)
	}

	if err := chownToSudoInvoker(path); err != nil {
		t.Fatalf("expected no-op as non-root, got: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat lockfile: %v", err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("unexpected Stat_t type %T", info.Sys())
	}
	if int(stat.Uid) != os.Geteuid() {
		t.Fatalf("lockfile uid changed to %d; chown must not run as non-root", stat.Uid)
	}
}

func TestAcquire_NonRootWithSudoEnvSucceeds(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("euid gate is only observable as non-root")
	}
	t.Setenv("SUDO_UID", "0")
	t.Setenv("SUDO_GID", "0")

	lock, err := Acquire(t.TempDir(), "deploy")
	if err != nil {
		t.Fatalf("acquire failed with sudo env set as non-root: %v", err)
	}
	lock.Release()
}
