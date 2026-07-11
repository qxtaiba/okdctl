package hostssh

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

// installFakeSSH writes a POSIX shell script named "ssh" in a temp dir and
// prepends that dir to PATH. Behaviour switches on SSH_FAKE_MODE. The test
// executor must use WithInheritedEnv because "SSH_" is not a prefix in
// DefaultEnvAllowlist, so SSH_FAKE_MODE would otherwise be filtered out.
func installFakeSSH(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-ssh script relies on POSIX sh")
	}
	dir := t.TempDir()
	script := `#!/bin/sh
# Fake ssh for testing — behaviour keyed off SSH_FAKE_MODE.
# SSHRun layout:     $1=-o $2=StrictHostKeyChecking=accept-new $3=-o
#                    $4=BatchMode=yes $5=root@host $6=<command-string>
# SSHRunArgv layout: ... $5=root@host $6=pvesh $7=get $8=<path> ...
case "$6" in
  pvesh)
    case "$8" in
      */config)
        case "${SSH_FAKE_MODE:-no-ref}" in
          in-use)
            printf '{"ide2":"local:iso/fedora-coreos-40.20240101.iso,media=cdrom","scsi0":"local-lvm:vm-101-disk-0,size=120G"}'
            ;;
          *)
            printf '{"scsi0":"local-lvm:vm-101-disk-0,size=120G","memory":4096}'
            ;;
        esac
        ;;
      *)
        printf '[{"vmid":101,"status":"running","name":"okd-master-0","mem":16384,"cpus":8,"uptime":3600}]'
        ;;
    esac
    ;;
  find*)
    case "${SSH_FAKE_MODE:-no-ref}" in
      unsafe-path)
        printf '/etc/passwd\0'
        ;;
      *)
        printf '/var/lib/vz/template/iso/fedora-coreos-40.20240101.iso\0'
        ;;
    esac
    ;;
  rm*)
    f="${SSH_RM_COUNTER}"
    if [ -n "$f" ]; then
      n=$(cat "$f" 2>/dev/null || printf '0')
      n=$((n + 1))
      printf '%d' "$n" > "$f"
    fi
    ;;
esac
exit 0
`
	path := filepath.Join(dir, "ssh")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func newTestISOParams(t *testing.T) *RemoteISOParams {
	t.Helper()
	return &RemoteISOParams{
		Host: "pve-test",
		Node: "pve-01",
		Exec: executor.New(executor.WithInheritedEnv()),
		Log:  logutil.NopLogger,
	}
}

func TestRemoveFCOSISOFromProxmox_removesUnreferencedISO(t *testing.T) {
	installFakeSSH(t)
	dir := t.TempDir()
	counter := filepath.Join(dir, "rm-counter")
	t.Setenv("SSH_FAKE_MODE", "no-ref")
	t.Setenv("SSH_RM_COUNTER", counter)

	p := newTestISOParams(t)
	if err := RemoveFCOSISOFromProxmox(context.Background(), p, "/var/lib/vz/template/iso"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	raw, readErr := os.ReadFile(counter)
	if readErr != nil {
		t.Fatalf("rm counter file not written — rm was not called: %v", readErr)
	}
	if string(raw) != "1" {
		t.Errorf("rm counter = %q; want 1", string(raw))
	}
}

func TestRemoveFCOSISOFromProxmox_skipsISOReferencedByRunningVM(t *testing.T) {
	installFakeSSH(t)
	dir := t.TempDir()
	counter := filepath.Join(dir, "rm-counter")
	t.Setenv("SSH_FAKE_MODE", "in-use")
	t.Setenv("SSH_RM_COUNTER", counter)

	p := newTestISOParams(t)
	if err := RemoveFCOSISOFromProxmox(context.Background(), p, "/var/lib/vz/template/iso"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, statErr := os.Stat(counter); statErr == nil {
		raw, _ := os.ReadFile(counter)
		t.Errorf("rm called (counter=%q) but ISO is in use — removal must be skipped", string(raw))
	}
}

func TestRemoveFCOSISOFromProxmox_rejectsUnsafePath(t *testing.T) {
	installFakeSSH(t)
	dir := t.TempDir()
	counter := filepath.Join(dir, "rm-counter")
	t.Setenv("SSH_FAKE_MODE", "unsafe-path")
	t.Setenv("SSH_RM_COUNTER", counter)

	p := newTestISOParams(t)
	if err := RemoveFCOSISOFromProxmox(context.Background(), p, "/var/lib/vz/template/iso"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, statErr := os.Stat(counter); statErr == nil {
		raw, _ := os.ReadFile(counter)
		t.Errorf("rm called (counter=%q) but find returned /etc/passwd — refuseUnsafeISOPath must block rm", string(raw))
	}
}
