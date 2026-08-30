package hostssh

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/testutil"
)

// installFakeSSH stubs ssh via PATH, keyed off SSH_FAKE_MODE; the executor
// needs WithInheritedEnv since "SSH_" isn't in DefaultEnvAllowlist.
func installFakeSSH(t *testing.T) {
	t.Helper()
	script := `#!/bin/sh
# Fake ssh for testing — behaviour keyed off SSH_FAKE_MODE.
# SSHRun layout:     $1=-o $2=StrictHostKeyChecking=accept-new $3=-o
#                    $4=BatchMode=yes $5=-o $6=ConnectTimeout=10
#                    $7=root@host $8=<command-string>
# SSHRunArgv layout: ... $7=root@host $8=pvesh $9=get $10=<path> ...
case "$8" in
  pvesh)
    case "${10}" in
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
	testutil.InstallFakeBin(t, "ssh", script)
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

func setupISORemoveFake(t *testing.T, mode string) (string, *RemoteISOParams) {
	t.Helper()
	installFakeSSH(t)
	counter := filepath.Join(t.TempDir(), "rm-counter")
	t.Setenv("SSH_FAKE_MODE", mode)
	t.Setenv("SSH_RM_COUNTER", counter)
	return counter, newTestISOParams(t)
}

// assertRMCalls: wantRM "" means rm must never run; any other value is the exact call count.
func assertRMCalls(t *testing.T, counter, wantRM, blocked string) {
	t.Helper()
	if wantRM == "" {
		if _, statErr := os.Stat(counter); statErr == nil {
			raw, _ := os.ReadFile(counter)
			t.Errorf("rm called (counter=%q) but %s — removal must be skipped", string(raw), blocked)
		}
		return
	}
	raw, readErr := os.ReadFile(counter)
	if readErr != nil {
		t.Fatalf("rm counter file not written — rm was not called: %v", readErr)
	}
	if string(raw) != wantRM {
		t.Errorf("rm counter = %q; want %s", string(raw), wantRM)
	}
}

func TestRemoveFCOSISOFromProxmox(t *testing.T) {
	cases := []struct {
		name    string
		mode    string
		wantRM  string // "" means rm must never run
		blocked string
	}{
		{name: "removes unreferenced iso", mode: "no-ref", wantRM: "1"},
		{name: "skips iso referenced by running vm", mode: "in-use", blocked: "ISO is in use"},
		{
			name: "rejects unsafe path", mode: "unsafe-path",
			blocked: "find returned /etc/passwd (refuseUnsafeISOPath must block rm)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			counter, p := setupISORemoveFake(t, tc.mode)
			if err := RemoveFCOSISOFromProxmox(context.Background(), p, "/var/lib/vz/template/iso"); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertRMCalls(t, counter, tc.wantRM, tc.blocked)
		})
	}
}
