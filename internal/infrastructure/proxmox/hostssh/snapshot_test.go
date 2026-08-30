package hostssh

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/testutil"
)

func TestValidateVMID(t *testing.T) {
	accept := []int{100, 101, 999999999, 12345}
	for _, vmid := range accept {
		if err := validateVMID(vmid); err != nil {
			t.Errorf("validateVMID(%d) rejected; want nil: %v", vmid, err)
		}
	}

	reject := []int{0, 1, 99, -1, 1000000000, 2000000000}
	for _, vmid := range reject {
		if err := validateVMID(vmid); err == nil {
			t.Errorf("validateVMID(%d) accepted; want error", vmid)
		}
	}
}

// Reject list mirrors TestValidateProxmoxName's injection payloads — same guard
// purpose, different atom.
func TestValidateSnapshotName(t *testing.T) {
	accept := []string{"a", "A", "snap-1", "snap_1", "okdctl-20260713-101500"}
	for _, name := range accept {
		if err := ValidateSnapshotName(name); err != nil {
			t.Errorf("ValidateSnapshotName(%q) rejected; want nil: %v", name, err)
		}
	}

	reject := []string{
		"",
		"1snap",
		"-snap",
		"_snap",
		"snap.name",
		"snap/name",
		"A" + strings.Repeat("a", 40), // 41 chars, over the 40-char cap
		"snap`id`",
		"snap$(reboot)",
		"snap;rm",
		"snap|pipe",
		"snap&bg",
		"snap name",
		"snap\tname",
	}
	for _, name := range reject {
		if err := ValidateSnapshotName(name); err == nil {
			t.Errorf("ValidateSnapshotName(%q) accepted; want error", name)
		}
	}
}

// Reject list mirrors injection payloads plus whitespace, which would
// word-split in SSHRunArgv's space-join.
func TestValidateSnapshotDescription(t *testing.T) {
	accept := []string{"", "pre-upgrade_snapshot", "before-ceph-rebuild,2026-07-13"}
	for _, desc := range accept {
		if err := ValidateSnapshotDescription(desc); err != nil {
			t.Errorf("ValidateSnapshotDescription(%q) rejected; want nil: %v", desc, err)
		}
	}

	longDesc := strings.Repeat("a", 201)
	reject := []string{
		"desc\nname", "desc\x00", longDesc,
		// "-vmstate"/"--description": a dash-led description could otherwise
		// read as a pvesh option token on the remote command line.
		"-vmstate", "--description", "_leading", ".leading",
		// Injection payloads.
		"desc`id`", "desc$(reboot)", "desc;rm", "desc|pipe", "desc&bg",
		// Whitespace word-splits in ssh's space-join.
		"before upgrade", "before\tupgrade", "before\nupgrade", " leading", "trailing ",
	}
	for _, desc := range reject {
		if err := ValidateSnapshotDescription(desc); err == nil {
			t.Errorf("ValidateSnapshotDescription(%q) accepted; want error", desc)
		}
	}
}

// Mirrors the injection payload set — validateUPID guards a server-returned
// value, not just caller input.
func TestValidateUPID(t *testing.T) {
	accept := []string{
		"UPID:pve-01:0002ABCD:00112233:00445566:qmsnapshot:100:root@pam:",
	}
	for _, upid := range accept {
		if err := validateUPID(upid); err != nil {
			t.Errorf("validateUPID(%q) rejected; want nil: %v", upid, err)
		}
	}

	reject := []string{
		"",
		"UPID`id`",
		"UPID$(reboot)",
		"UPID;rm",
		"UPID|pipe",
		"UPID&bg",
		"UPID name",
	}
	for _, upid := range reject {
		if err := validateUPID(upid); err == nil {
			t.Errorf("validateUPID(%q) accepted; want error", upid)
		}
	}
}

func TestAgentFlagEnabled(t *testing.T) {
	cases := map[string]bool{
		"1":                               true,
		"0":                               false,
		"":                                false,
		"enabled=1":                       true,
		"enabled=0":                       false,
		"enabled=1,fstrim_cloned_disks=1": true,
		"enabled=0,fstrim_cloned_disks=1": false,
		"garbage":                         false,
	}
	for raw, want := range cases {
		if got := agentFlagEnabled(raw); got != want {
			t.Errorf("agentFlagEnabled(%q) = %v; want %v", raw, got, want)
		}
	}
}

func TestParseSnapshotList(t *testing.T) {
	t.Run("filters current", func(t *testing.T) {
		raw := `[
			{"name":"current","description":"You are here!"},
			{"name":"pre-upgrade","description":"before upgrade","snaptime":1690000000,"parent":""},
			{"name":"post-upgrade","description":"after upgrade","snaptime":1690003600,"parent":"pre-upgrade"}
		]`

		got, err := parseSnapshotList(raw)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("len(got) = %d; want 2 (current filtered out)", len(got))
		}
		if got[0] != (SnapshotInfo{Name: "pre-upgrade", Description: "before upgrade", SnapTime: 1690000000}) {
			t.Errorf("got[0] = %+v", got[0])
		}
		if got[1] != (SnapshotInfo{Name: "post-upgrade", Description: "after upgrade", SnapTime: 1690003600, Parent: "pre-upgrade"}) {
			t.Errorf("got[1] = %+v", got[1])
		}
	})

	t.Run("empty list", func(t *testing.T) {
		got, err := parseSnapshotList(`[]`)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("len(got) = %d; want 0", len(got))
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		if _, err := parseSnapshotList("not json"); err == nil {
			t.Fatal("expected error for malformed json; got nil")
		}
	})
}

// Confirms parseUPID validates server-controlled output too, not just caller input.
func TestParseUPID(t *testing.T) {
	got, err := parseUPID(`"UPID:pve-01:0002ABCD:00112233:00445566:qmsnapshot:100:root@pam:"`)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := "UPID:pve-01:0002ABCD:00112233:00445566:qmsnapshot:100:root@pam:"
	if got != want {
		t.Errorf("parseUPID = %q; want %q", got, want)
	}

	if _, err := parseUPID("not json"); err == nil {
		t.Fatal("expected error for malformed json; got nil")
	}
	if _, err := parseUPID(`"UPID:pve;rm -rf /"`); err == nil {
		t.Fatal("expected error for injection payload in upid; got nil")
	}
}

// installFakeSnapshotSSH fakes pvesh create/delete/get over SSHRunArgvOutput's
// positional args ($8=pvesh $9=subcommand $10=path), keyed off SNAP_* env
// vars; distinct from installFakeSSH (remove_fcos_iso_test.go), which only
// covers ISO-cleanup shapes.
func installFakeSnapshotSSH(t *testing.T) {
	t.Helper()
	script := `#!/bin/sh
if [ -n "${SNAP_ARGV_LOG:-}" ]; then
  echo "$@" >> "$SNAP_ARGV_LOG"
fi
case "$9" in
  create|delete)
    if [ -n "${SNAP_TASK_EXIT_CODE:-}" ] && [ "${SNAP_TASK_EXIT_CODE}" != "0" ]; then
      echo "${SNAP_TASK_STDERR:-pvesh rejected the request}" >&2
      exit "${SNAP_TASK_EXIT_CODE}"
    fi
    printf '"%s"' "${SNAP_UPID:-UPID:pve-01:00000001:00000001:00000001:qmsnapshot:100:root@pam:}"
    exit 0
    ;;
  get)
    case "${10}" in
      */status)
        f="${SNAP_POLL_COUNTER:-}"
        if [ -n "$f" ]; then
          n=$(cat "$f" 2>/dev/null || printf '0')
          n=$((n + 1))
          printf '%d' "$n" > "$f"
        else
          n=1
        fi
        if [ "$n" -ge "${SNAP_STOPPED_AT:-1}" ]; then
          printf '{"status":"stopped","exitstatus":"%s"}' "${SNAP_EXITSTATUS:-OK}"
        else
          printf '{"status":"running"}'
        fi
        exit 0
        ;;
      */config)
        printf '{"agent":"%s"}' "${SNAP_AGENT_VALUE:-}"
        exit 0
        ;;
      *)
        cat "${SNAP_LIST_FILE:-/dev/null}"
        exit 0
        ;;
    esac
    ;;
esac
exit 1
`
	testutil.InstallFakeBin(t, "ssh", script)
}

func newTestSnapshotParams(t *testing.T) *RemoteISOParams {
	t.Helper()
	return &RemoteISOParams{
		Host: "pve-test",
		Node: "pve-01",
		Exec: executor.New(executor.WithInheritedEnv()),
		Log:  logutil.NopLogger,
	}
}

// qemu-guest-agent is disabled fleet-wide, so a memory-state snapshot would not
// be crash-consistent.
func TestCreateSnapshot_neverPassesVMState(t *testing.T) {
	installFakeSnapshotSSH(t)
	p := newTestSnapshotParams(t)
	log := filepath.Join(t.TempDir(), "argv.log")
	t.Setenv("SNAP_ARGV_LOG", log)

	if err := CreateSnapshot(context.Background(), p, 100, "pre-upgrade", "before-upgrade", 5*time.Second); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("argv log not written: %v", err)
	}
	argv := string(raw)
	if strings.Contains(argv, "-vmstate") {
		t.Errorf("argv %q contains -vmstate; must never be passed", argv)
	}
	if !strings.Contains(argv, "-snapname pre-upgrade") {
		t.Errorf("argv %q missing -snapname pre-upgrade", argv)
	}
	if !strings.Contains(argv, "-description before-upgrade") {
		t.Errorf("argv %q missing -description", argv)
	}
}

func TestCreateSnapshot_taskExitStatusNotOK(t *testing.T) {
	installFakeSnapshotSSH(t)
	p := newTestSnapshotParams(t)
	t.Setenv("SNAP_EXITSTATUS", "snapshot failed - no space left on device")

	err := CreateSnapshot(context.Background(), p, 100, "pre-upgrade", "", 5*time.Second)
	if err == nil {
		t.Fatal("expected error for non-OK exitstatus; got nil")
	}
	if !strings.Contains(err.Error(), "no space left on device") {
		t.Errorf("err = %q; want it to name the exitstatus", err.Error())
	}
	if strings.Contains(err.Error(), "timeout") {
		t.Errorf("err = %q; a stopped+failed task must not read as a timeout", err.Error())
	}
}

// pveshRun would silently swallow a non-zero exit; unacceptable for a
// destructive write. Also pins executor.ExitError routing: stderr surfaces but
// scrubbed, the path stays out of the Command label and survives in the wrap.
func TestCreateSnapshot_rejectedByPveshScrubsStderr(t *testing.T) {
	installFakeSnapshotSSH(t)
	p := newTestSnapshotParams(t)
	t.Setenv("SNAP_TASK_EXIT_CODE", "1")
	t.Setenv("SNAP_TASK_STDERR", "401 authentication failure PROXMOX_VE_PASSWORD=hunter2")

	err := CreateSnapshot(context.Background(), p, 100, "pre-upgrade", "", 5*time.Second)
	if err == nil {
		t.Fatal("expected error for rejected create call; got nil")
	}
	var exitErr *executor.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("err = %v (%T); want executor.ExitError in the chain", err, err)
	}
	if exitErr.Command != "pvesh create" {
		t.Errorf("ExitError.Command = %q; the path must stay out of the Command label", exitErr.Command)
	}
	if !strings.Contains(err.Error(), "401 authentication failure") {
		t.Errorf("err = %q; want it to surface pvesh stderr", err.Error())
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Errorf("err = %q; remote stderr credential leaked unscrubbed", err.Error())
	}
	if !strings.Contains(err.Error(), "/nodes/pve-01/qemu/100/snapshot") {
		t.Errorf("err = %q; want the pvesh path context in the message", err.Error())
	}
}

func TestCreateSnapshot_duplicateName(t *testing.T) {
	installFakeSnapshotSSH(t)
	p := newTestSnapshotParams(t)

	listFile := filepath.Join(t.TempDir(), "list.json")
	if err := os.WriteFile(listFile, []byte(`[{"name":"pre-upgrade","snaptime":1690000000}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SNAP_LIST_FILE", listFile)

	err := CreateSnapshot(context.Background(), p, 100, "pre-upgrade", "", 5*time.Second)
	if !errors.Is(err, errSnapshotExists) {
		t.Fatalf("err = %v; want errors.Is(err, errSnapshotExists)", err)
	}
	if !strings.Contains(err.Error(), "pre-upgrade") {
		t.Errorf("err = %q; want it to name the colliding snapshot", err.Error())
	}
}

func TestCreateSnapshot_otherSnapshotsDoNotCollide(t *testing.T) {
	installFakeSnapshotSSH(t)
	p := newTestSnapshotParams(t)

	listFile := filepath.Join(t.TempDir(), "list.json")
	if err := os.WriteFile(listFile, []byte(`[{"name":"post-upgrade","snaptime":1690000000}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SNAP_LIST_FILE", listFile)

	if err := CreateSnapshot(context.Background(), p, 100, "pre-upgrade", "", 5*time.Second); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Guards against declaring success the moment pvesh returns a UPID (fire-and-forget failure mode).
func TestCreateSnapshot_timesOutWhenTaskNeverStops(t *testing.T) {
	installFakeSnapshotSSH(t)
	p := newTestSnapshotParams(t)
	t.Setenv("SNAP_STOPPED_AT", "999999")

	err := CreateSnapshot(context.Background(), p, 100, "pre-upgrade", "", 20*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error when task never reaches stopped; got nil")
	}
}

// No fake ssh installed — reaching the transport would fail differently,
// proving validation runs first.
func TestSnapshotOps_invalidArgs(t *testing.T) {
	p := &RemoteISOParams{Node: "pve-01"}
	cases := []struct {
		name string
		call func(ctx context.Context) error
	}{
		{"create invalid vmid", func(ctx context.Context) error {
			return CreateSnapshot(ctx, p, 1, "pre-upgrade", "", time.Second)
		}},
		{"create invalid name", func(ctx context.Context) error {
			return CreateSnapshot(ctx, p, 100, "snap;rm", "", time.Second)
		}},
		{"create invalid description", func(ctx context.Context) error {
			return CreateSnapshot(ctx, p, 100, "pre-upgrade", "desc`id`", time.Second)
		}},
		{"list invalid vmid", func(ctx context.Context) error {
			_, err := ListSnapshots(ctx, p, 0)
			return err
		}},
		{"rollback invalid name", func(ctx context.Context) error {
			return RollbackSnapshot(ctx, p, 100, "$(reboot)", time.Second)
		}},
		{"delete empty name", func(ctx context.Context) error {
			return DeleteSnapshot(ctx, p, 100, "", time.Second)
		}},
		{"agent probe invalid vmid", func(ctx context.Context) error {
			_, err := VMAgentEnabled(ctx, p, -1)
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(context.Background()); err == nil {
				t.Fatal("expected validation error; got nil")
			}
		})
	}
}

// -start 1 auto-starts the VM post-rollback, even one deliberately powered off.
func TestRollbackSnapshot_passesStart1(t *testing.T) {
	installFakeSnapshotSSH(t)
	p := newTestSnapshotParams(t)
	log := filepath.Join(t.TempDir(), "argv.log")
	t.Setenv("SNAP_ARGV_LOG", log)

	if err := RollbackSnapshot(context.Background(), p, 100, "pre-upgrade", 5*time.Second); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("argv log not written: %v", err)
	}
	argv := string(raw)
	if !strings.Contains(argv, "snapshot/pre-upgrade/rollback") {
		t.Errorf("argv %q missing rollback path", argv)
	}
	if !strings.Contains(argv, "-start 1") {
		t.Errorf("argv %q missing -start 1", argv)
	}
}

func TestDeleteSnapshot_success(t *testing.T) {
	installFakeSnapshotSSH(t)
	p := newTestSnapshotParams(t)

	if err := DeleteSnapshot(context.Background(), p, 100, "pre-upgrade", 5*time.Second); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Wiring only — the agent-value shapes themselves are covered by TestAgentFlagEnabled.
func TestVMAgentEnabled(t *testing.T) {
	installFakeSnapshotSSH(t)
	p := newTestSnapshotParams(t)
	t.Setenv("SNAP_AGENT_VALUE", "enabled=1")

	got, err := VMAgentEnabled(context.Background(), p, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Error("VMAgentEnabled with agent=enabled=1 = false; want true")
	}
}
