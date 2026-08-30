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
	"github.com/qxtaiba/okdctl/internal/testutil"
)

func TestSnapshotState_WritesBackup(t *testing.T) {
	dir := t.TempDir()
	payload := []byte(`{"version":4,"resources":[{"type":"proxmox_vm_qemu"}]}`)
	mustWriteFile(t, dir, "terraform.tfstate", string(payload))

	e := &Executor{workDir: dir, logger: slog.New(&testutil.CaptureHandler{})}
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
	e := &Executor{workDir: dir, logger: slog.New(&testutil.CaptureHandler{})}

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
	mustWriteFile(t, dir, "terraform.tfstate", `{"version":4,"resources":[]}`)

	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	e := &Executor{workDir: dir, logger: slog.New(&testutil.CaptureHandler{})}
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
	names := writeBaks(t, dir, 1, 2, 3, 4, 5, 6, 7)

	e := &Executor{workDir: dir, logger: slog.New(&testutil.CaptureHandler{})}
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

func TestCheckStateMajorVersion(t *testing.T) {
	cases := []struct {
		name          string
		version       string
		wantConfigErr bool
	}{
		{"rejects major two", "2.0.0", true},
		{"unparseable version is non-fatal", "notasemver", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			sf := mustWriteFile(t, dir, "terraform.tfstate",
				`{"version":4,"terraform_version":"`+tc.version+`","resources":[]}`)

			err := checkStateMajorVersion(sf, slog.New(&testutil.CaptureHandler{}))
			if !tc.wantConfigErr {
				if err != nil {
					t.Errorf("expected nil for version %q; got %v", tc.version, err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected ConfigError for major=2; got nil")
			}
			var ce *errtypes.ConfigError
			if !errors.As(err, &ce) {
				t.Errorf("expected *errtypes.ConfigError; got %T: %v", err, err)
			}
		})
	}
}

func TestExecutor_StateHasResource(t *testing.T) {
	addr := "module.okd_cluster.proxmox_virtual_environment_vm.worker[2]"

	cases := []struct {
		name    string
		script  string
		want    bool
		wantErr bool
	}{
		{
			name:   "exit 1 with empty stdout and stderr means absent",
			script: "#!/bin/sh\nexit 1\n",
			want:   false,
		},
		{
			name:    "exit 1 with stderr output is a hard error",
			script:  "#!/bin/sh\necho 'Error: Failed to load state' >&2\nexit 1\n",
			wantErr: true,
		},
		{
			name:   "exit 0 means present",
			script: "#!/bin/sh\necho \"$3\"\nexit 0\n",
			want:   true,
		},
		{
			name:    "other non-zero exit is a hard error, never silent absence",
			script:  "#!/bin/sh\nexit 2\n",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testutil.InstallFakeBin(t, "terraform", tc.script)
			e := New(t.TempDir())

			got, err := e.StateHasResource(context.Background(), addr)
			if tc.wantErr {
				if err == nil {
					t.Fatal("StateHasResource: expected error; got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("StateHasResource: unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("StateHasResource() = %v; want %v", got, tc.want)
			}
		})
	}
}
