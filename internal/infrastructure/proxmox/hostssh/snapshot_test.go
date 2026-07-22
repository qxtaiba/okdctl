package hostssh

import (
	"context"
	"encoding/json"
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

// TestValidateSnapshotName_RejectsInjectionPayloads exercises the exact
// shell-metacharacter payloads TestValidateProxmoxName_RejectsBadNode uses
// in pvesh_test.go: validateSnapshotName is the injection guard for the
// snapshot-create/rollback/delete argv path the same way validateProxmoxName
// guards the node atom.
func TestValidateSnapshotName_RejectsInjectionPayloads(t *testing.T) {
	payloads := []string{
		"snap`id`",
		"snap$(reboot)",
		"snap;rm",
		"snap|pipe",
		"snap&bg",
		"snap name",
		"snap\tname",
	}
	for _, name := range payloads {
		if err := validateSnapshotName(name); err == nil {
			t.Errorf("validateSnapshotName(%q) accepted; want error", name)
		}
	}
}

func TestValidateSnapshotName(t *testing.T) {
	accept := []string{"a", "A", "snap-1", "snap_1", "okdctl-20260713-101500"}
	for _, name := range accept {
		if err := validateSnapshotName(name); err != nil {
			t.Errorf("validateSnapshotName(%q) rejected; want nil: %v", name, err)
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
	}
	for _, name := range reject {
		if err := validateSnapshotName(name); err == nil {
			t.Errorf("validateSnapshotName(%q) accepted; want error", name)
		}
	}
}

// TestValidateSnapshotDescription_RejectsInjectionPayloads mirrors
// TestValidateSnapshotName_RejectsInjectionPayloads: description reaches the
// remote shell as its own SSHRunArgv atom, so it needs the same guard.
func TestValidateSnapshotDescription_RejectsInjectionPayloads(t *testing.T) {
	payloads := []string{
		"desc`id`",
		"desc$(reboot)",
		"desc;rm",
		"desc|pipe",
		"desc&bg",
	}
	for _, desc := range payloads {
		if err := validateSnapshotDescription(desc); err == nil {
			t.Errorf("validateSnapshotDescription(%q) accepted; want error", desc)
		}
	}
}

func TestValidateSnapshotDescription(t *testing.T) {
	accept := []string{"", "pre-upgrade_snapshot", "before-ceph-rebuild,2026-07-13"}
	for _, desc := range accept {
		if err := validateSnapshotDescription(desc); err != nil {
			t.Errorf("validateSnapshotDescription(%q) rejected; want nil: %v", desc, err)
		}
	}

	longDesc := strings.Repeat("a", 201)
	reject := []string{"desc\nname", "desc\x00", longDesc}
	for _, desc := range reject {
		if err := validateSnapshotDescription(desc); err == nil {
			t.Errorf("validateSnapshotDescription(%q) accepted; want error", desc)
		}
	}
}

// TestValidateSnapshotDescription_RejectsWhitespace is FIX 1: SSHRunArgv
// space-joins argv before handing it to the remote login shell, so a
// multi-word description would word-split there instead of surviving as the
// single token pvesh's -description flag expects. The validator must fail
// closed rather than let a spaced value reach the remote shell corrupted.
func TestValidateSnapshotDescription_RejectsWhitespace(t *testing.T) {
	reject := []string{"before upgrade", "before\tupgrade", "before\nupgrade", " leading", "trailing "}
	for _, desc := range reject {
		if err := validateSnapshotDescription(desc); err == nil {
			t.Errorf("validateSnapshotDescription(%q) accepted; want error (whitespace must be rejected)", desc)
		}
	}
}

// TestValidateUPID_RejectsInjectionPayloads mirrors the pvesh_test.go
// payload set: validateUPID guards the task-status path built from a UPID
// that Proxmox itself returned, so an attacker able to forge the response
// (or trigger a parse bug) must not be able to smuggle metacharacters
// through pveshWaitTask's SSHRunArgv call.
func TestValidateUPID_RejectsInjectionPayloads(t *testing.T) {
	payloads := []string{
		"UPID`id`",
		"UPID$(reboot)",
		"UPID;rm",
		"UPID|pipe",
		"UPID&bg",
		"UPID name",
	}
	for _, upid := range payloads {
		if err := validateUPID(upid); err == nil {
			t.Errorf("validateUPID(%q) accepted; want error", upid)
		}
	}
}

func TestValidateUPID(t *testing.T) {
	accept := []string{
		"UPID:pve-01:0002ABCD:00112233:00445566:qmsnapshot:100:root@pam:",
	}
	for _, upid := range accept {
		if err := validateUPID(upid); err != nil {
			t.Errorf("validateUPID(%q) rejected; want nil: %v", upid, err)
		}
	}

	reject := []string{""}
	for _, upid := range reject {
		if err := validateUPID(upid); err == nil {
			t.Errorf("validateUPID(%q) accepted; want error", upid)
		}
	}
}

func TestPveshSnapshotPath(t *testing.T) {
	got := pveshSnapshotPath("pve-01", 100)
	want := "/nodes/pve-01/qemu/100/snapshot"
	if got != want {
		t.Errorf("pveshSnapshotPath = %q; want %q", got, want)
	}
}

func TestPveshSnapshotNamePath(t *testing.T) {
	got := pveshSnapshotNamePath("pve-01", 100, "pre-upgrade")
	want := "/nodes/pve-01/qemu/100/snapshot/pre-upgrade"
	if got != want {
		t.Errorf("pveshSnapshotNamePath = %q; want %q", got, want)
	}
}

func TestPveshTaskStatusPath(t *testing.T) {
	got := pveshTaskStatusPath("pve-01", "UPID:pve-01:0002ABCD:00112233:00445566:qmsnapshot:100:root@pam:")
	want := "/nodes/pve-01/tasks/UPID:pve-01:0002ABCD:00112233:00445566:qmsnapshot:100:root@pam:/status"
	if got != want {
		t.Errorf("pveshTaskStatusPath = %q; want %q", got, want)
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
}

func TestParseSnapshotList_EmptyList(t *testing.T) {
	got, err := parseSnapshotList(`[]`)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d; want 0", len(got))
	}
}

func TestParseSnapshotList_InvalidJSON(t *testing.T) {
	if _, err := parseSnapshotList("not json"); err == nil {
		t.Fatal("expected error for malformed json; got nil")
	}
}

func TestTaskStatusUnmarshal(t *testing.T) {
	var running taskStatus
	if err := json.Unmarshal([]byte(`{"status":"running"}`), &running); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if running.Status != "running" || running.ExitStatus != "" {
		t.Errorf("running = %+v", running)
	}

	var stoppedOK taskStatus
	if err := json.Unmarshal([]byte(`{"status":"stopped","exitstatus":"OK"}`), &stoppedOK); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if stoppedOK.Status != taskStatusStopped || stoppedOK.ExitStatus != "OK" {
		t.Errorf("stoppedOK = %+v", stoppedOK)
	}

	var stoppedFailed taskStatus
	if err := json.Unmarshal([]byte(`{"status":"stopped","exitstatus":"snapshot failed - no space left on device"}`), &stoppedFailed); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if stoppedFailed.Status != taskStatusStopped || stoppedFailed.ExitStatus == "OK" {
		t.Errorf("stoppedFailed = %+v", stoppedFailed)
	}
}

func TestParseUPID(t *testing.T) {
	got, err := parseUPID(`"UPID:pve-01:0002ABCD:00112233:00445566:qmsnapshot:100:root@pam:"`)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := "UPID:pve-01:0002ABCD:00112233:00445566:qmsnapshot:100:root@pam:"
	if got != want {
		t.Errorf("parseUPID = %q; want %q", got, want)
	}
}

func TestParseUPID_InvalidJSON(t *testing.T) {
	if _, err := parseUPID("not json"); err == nil {
		t.Fatal("expected error for malformed json; got nil")
	}
}

// TestParseUPID_RejectsInjection confirms parseUPID applies validateUPID to
// server-controlled output, not just caller-controlled input — a
// compromised or spoofed pvesh response must not smuggle metacharacters
// past this boundary either.
func TestParseUPID_RejectsInjection(t *testing.T) {
	if _, err := parseUPID(`"UPID:pve;rm -rf /"`); err == nil {
		t.Fatal("expected error for injection payload in upid; got nil")
	}
}

// installFakeSnapshotSSH writes a POSIX shell script named "ssh" that fakes
// pvesh create/delete/get responses for snapshot operations, keyed off
// SNAP_* env vars. Distinct from installFakeSSH (remove_fcos_iso_test.go),
// whose argv matching only covers the ISO-cleanup call shapes.
//
// SSHRunArgvOutput layout for accept-new mode: $1=-o $2=... $3=-o $4=...
// $5=root@host $6=pvesh $7=<subcommand> $8=<path> [extra...].
func installFakeSnapshotSSH(t *testing.T) {
	t.Helper()
	script := `#!/bin/sh
if [ -n "${SNAP_ARGV_LOG:-}" ]; then
  echo "$@" >> "$SNAP_ARGV_LOG"
fi
case "$7" in
  create|delete)
    if [ -n "${SNAP_TASK_EXIT_CODE:-}" ] && [ "${SNAP_TASK_EXIT_CODE}" != "0" ]; then
      echo "${SNAP_TASK_STDERR:-pvesh rejected the request}" >&2
      exit "${SNAP_TASK_EXIT_CODE}"
    fi
    printf '"%s"' "${SNAP_UPID:-UPID:pve-01:00000001:00000001:00000001:qmsnapshot:100:root@pam:}"
    exit 0
    ;;
  get)
    case "$8" in
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

func TestCreateSnapshot_success(t *testing.T) {
	installFakeSnapshotSSH(t)
	p := newTestSnapshotParams(t)

	if err := CreateSnapshot(context.Background(), p, 100, "pre-upgrade", "before-upgrade", 5*time.Second); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestCreateSnapshot_neverPassesVMState asserts -vmstate is absent from the
// pvesh argv: the qemu-guest-agent is disabled fleet-wide, so a memory-state
// snapshot would not be crash-consistent and must never be requested.
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

// TestCreateSnapshot_taskExitStatusNotOK covers the pveshWaitTask distinction
// design calls out: a stopped task with a non-OK exitstatus must surface as
// its own error, not get folded into a generic timeout.
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

// TestCreateSnapshot_rejectedByPvesh covers pveshCreateTaskCall's ExitCode
// check: pveshRun would silently swallow a non-zero exit, which is
// unacceptable once the call is issuing a destructive write.
func TestCreateSnapshot_rejectedByPvesh(t *testing.T) {
	installFakeSnapshotSSH(t)
	p := newTestSnapshotParams(t)
	t.Setenv("SNAP_TASK_EXIT_CODE", "1")
	t.Setenv("SNAP_TASK_STDERR", "500 no such storage")

	err := CreateSnapshot(context.Background(), p, 100, "pre-upgrade", "", 5*time.Second)
	if err == nil {
		t.Fatal("expected error for rejected create call; got nil")
	}
	if !strings.Contains(err.Error(), "no such storage") {
		t.Errorf("err = %q; want it to surface pvesh stderr", err.Error())
	}
}

// TestCreateSnapshot_timesOutWhenTaskNeverStops proves pveshWaitTask does
// not treat "pvesh returned a UPID" as success: if the background task never
// reaches status=stopped, CreateSnapshot must fail rather than declare
// victory the moment the task was launched (the fire-and-forget failure
// mode the design explicitly guards against).
func TestCreateSnapshot_timesOutWhenTaskNeverStops(t *testing.T) {
	installFakeSnapshotSSH(t)
	p := newTestSnapshotParams(t)
	t.Setenv("SNAP_STOPPED_AT", "999999")

	err := CreateSnapshot(context.Background(), p, 100, "pre-upgrade", "", 20*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error when task never reaches stopped; got nil")
	}
}

func TestCreateSnapshot_invalidVMID(t *testing.T) {
	p := &RemoteISOParams{Node: "pve-01"}
	if err := CreateSnapshot(context.Background(), p, 1, "pre-upgrade", "", time.Second); err == nil {
		t.Fatal("expected error for invalid vmid; got nil")
	}
}

func TestCreateSnapshot_invalidName(t *testing.T) {
	p := &RemoteISOParams{Node: "pve-01"}
	if err := CreateSnapshot(context.Background(), p, 100, "snap;rm", "", time.Second); err == nil {
		t.Fatal("expected error for invalid snapshot name; got nil")
	}
}

func TestCreateSnapshot_invalidDescription(t *testing.T) {
	p := &RemoteISOParams{Node: "pve-01"}
	if err := CreateSnapshot(context.Background(), p, 100, "pre-upgrade", "desc`id`", time.Second); err == nil {
		t.Fatal("expected error for invalid description; got nil")
	}
}

func TestListSnapshots_success(t *testing.T) {
	installFakeSnapshotSSH(t)
	p := newTestSnapshotParams(t)

	listFile := filepath.Join(t.TempDir(), "list.json")
	if err := os.WriteFile(listFile, []byte(`[
		{"name":"current","description":"You are here!"},
		{"name":"pre-upgrade","description":"before upgrade","snaptime":1690000000,"parent":""}
	]`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SNAP_LIST_FILE", listFile)

	got, err := ListSnapshots(context.Background(), p, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Name != "pre-upgrade" {
		t.Errorf("got = %+v; want one entry named pre-upgrade", got)
	}
}

func TestListSnapshots_invalidVMID(t *testing.T) {
	p := &RemoteISOParams{Node: "pve-01"}
	if _, err := ListSnapshots(context.Background(), p, 0); err == nil {
		t.Fatal("expected error for invalid vmid; got nil")
	}
}

// TestRollbackSnapshot_passesStart1 asserts -start 1 reaches pvesh: it
// auto-starts the VM once rollback completes, including a VM that was
// deliberately powered off — a deliberate, documented side effect.
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

func TestRollbackSnapshot_invalidName(t *testing.T) {
	p := &RemoteISOParams{Node: "pve-01"}
	if err := RollbackSnapshot(context.Background(), p, 100, "$(reboot)", time.Second); err == nil {
		t.Fatal("expected error for invalid snapshot name; got nil")
	}
}

func TestDeleteSnapshot_success(t *testing.T) {
	installFakeSnapshotSSH(t)
	p := newTestSnapshotParams(t)

	if err := DeleteSnapshot(context.Background(), p, 100, "pre-upgrade", 5*time.Second); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteSnapshot_invalidName(t *testing.T) {
	p := &RemoteISOParams{Node: "pve-01"}
	if err := DeleteSnapshot(context.Background(), p, 100, "", time.Second); err == nil {
		t.Fatal("expected error for empty snapshot name; got nil")
	}
}

func TestVMAgentEnabled(t *testing.T) {
	cases := map[string]bool{
		"1":         true,
		"0":         false,
		"":          false,
		"enabled=1": true,
	}
	for value, want := range cases {
		t.Run(value, func(t *testing.T) {
			installFakeSnapshotSSH(t)
			p := newTestSnapshotParams(t)
			t.Setenv("SNAP_AGENT_VALUE", value)

			got, err := VMAgentEnabled(context.Background(), p, 100)
			if err != nil {
				t.Fatalf("agent=%q: unexpected error: %v", value, err)
			}
			if got != want {
				t.Errorf("VMAgentEnabled with agent=%q = %v; want %v", value, got, want)
			}
		})
	}
}

func TestVMAgentEnabled_invalidVMID(t *testing.T) {
	p := &RemoteISOParams{Node: "pve-01"}
	if _, err := VMAgentEnabled(context.Background(), p, -1); err == nil {
		t.Fatal("expected error for invalid vmid; got nil")
	}
}
