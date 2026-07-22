package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/proxmox/hostssh"
	"github.com/qxtaiba/okdctl/internal/tui"
)

// captureTuiStderr redirects tui's package-level stderr logger to a buffer
// around fn and restores it after — tui.Warn/Info write through a charmlog
// writer captured once at package init, not the live os.Stderr variable, so
// a plain os.Pipe swap (captureStderr) never sees these records.
func captureTuiStderr(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	if err := tui.ConfigureLoggers("info", "text", io.Discard, &buf, false); err != nil {
		t.Fatalf("ConfigureLoggers: %v", err)
	}
	t.Cleanup(func() {
		if err := tui.ConfigureLoggers("info", "text", os.Stdout, os.Stderr, false); err != nil {
			t.Errorf("restore loggers: %v", err)
		}
	})
	fn()
	return buf.String()
}

func TestNodeSnapshotGate_YesSkipsPromptButStillPairsConfirmCluster(t *testing.T) {
	// --yes with no --confirm-cluster must fail closed regardless of twoStage.
	_, err := nodeSnapshotGate(context.Background(), "node snapshot create", false, true, false, "", "prod", "")
	var cfgErr *errtypes.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("want *errtypes.ConfigError, got %v", err)
	}
}

func TestNodeSnapshotGate_YesWithMatchingConfirmClusterProceedsWithoutPrompt(t *testing.T) {
	// No stdin wired: if this tried to prompt, promptForLine would hit the
	// TTY guard and return an error, so a nil error here proves --yes short-
	// circuited before runNodeGate.
	ok, err := nodeSnapshotGate(context.Background(), "node snapshot create", false, true, false, "prod", "prod", "warn")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v; want true, nil", ok, err)
	}
}

func TestNodeSnapshotGate_DryRunSkipsPromptEvenWithoutYes(t *testing.T) {
	// dryRun=true, yes=false, no stdin wired — must not attempt to prompt.
	ok, err := nodeSnapshotGate(context.Background(), "node snapshot rollback", true, false, true, "", "prod", "warn")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v; want true, nil (dry-run never blocks on a prompt)", ok, err)
	}
}

func TestNodeSnapshotGate_CreateSingleStageYN(t *testing.T) {
	testStdinReader = strings.NewReader("y\n")
	t.Cleanup(func() { testStdinReader = nil })

	ok, err := nodeSnapshotGate(context.Background(), "node snapshot create", false, false, false, "", "prod", "crash-consistent warning")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v; want true, nil", ok, err)
	}
}

func TestNodeSnapshotGate_CreateSingleStageDeniesOnPlainNo(t *testing.T) {
	testStdinReader = strings.NewReader("n\n")
	t.Cleanup(func() { testStdinReader = nil })

	ok, err := nodeSnapshotGate(context.Background(), "node snapshot create", false, false, false, "", "prod", "")
	if err != nil {
		t.Fatalf("want nil error on decline, got %v", err)
	}
	if ok {
		t.Fatal("plain 'n' must deny the single-stage gate")
	}
}

func TestNodeSnapshotGate_RollbackTwoStageRequiresTypedName(t *testing.T) {
	// The single-stage 'y' answer must NOT be enough for rollback's two-stage
	// gate — this is the create-vs-rollback wiring distinction under test.
	testStdinReader = strings.NewReader("y\n")
	t.Cleanup(func() { testStdinReader = nil })

	ok, err := nodeSnapshotGate(context.Background(), "node snapshot rollback", true, false, false, "", "prod", "")
	if err != nil {
		t.Fatalf("want nil error on deny, got %v", err)
	}
	if ok {
		t.Fatal("rollback's two-stage gate must not accept a bare y/N answer for the typed-name step")
	}
}

func TestNodeSnapshotGate_RollbackTwoStageHappyPath(t *testing.T) {
	pr, pw := io.Pipe()
	testStdinReader = pr
	t.Cleanup(func() {
		_ = pr.Close()
		testStdinReader = nil
	})
	go func() {
		_, _ = pw.Write([]byte("prod\n"))
		_, _ = pw.Write([]byte("y\n"))
		_ = pw.Close()
	}()

	ok, err := nodeSnapshotGate(context.Background(), "node snapshot rollback", true, false, false, "", "prod", "quorum-sensitive warning")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v; want true, nil", ok, err)
	}
}

func TestNodeSnapshotGate_WarnMsgOnlyPrintsOnInteractivePath(t *testing.T) {
	testStdinReader = strings.NewReader("y\n")
	t.Cleanup(func() { testStdinReader = nil })

	out := captureTuiStderr(t, func() {
		_, _ = nodeSnapshotGate(context.Background(), "node snapshot create", false, false, false, "", "prod", "crash-consistent only")
	})
	if !strings.Contains(out, "crash-consistent only") {
		t.Errorf("interactive gate must print warnMsg; got:\n%s", out)
	}
}

func TestNodeSnapshotGate_WarnMsgSuppressedUnderYes(t *testing.T) {
	out := captureTuiStderr(t, func() {
		_, _ = nodeSnapshotGate(context.Background(), "node snapshot create", false, true, false, "prod", "prod", "crash-consistent only")
	})
	if strings.Contains(out, "crash-consistent only") {
		t.Errorf("--yes must not print the CLI-level warnMsg (only the runner's own log line): got:\n%s", out)
	}
}

func TestToNodeSnapshotEntries(t *testing.T) {
	in := []hostssh.SnapshotInfo{
		{Name: "pre-upgrade", Description: "before upgrading to 4.21", SnapTime: 1735689600, Parent: ""},
		{Name: "current-ish", SnapTime: 0},
	}
	got := toNodeSnapshotEntries(in)
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].Name != "pre-upgrade" || got[0].Description != "before upgrading to 4.21" {
		t.Errorf("got[0] = %+v", got[0])
	}
	if got[0].SnapTime != "2025-01-01T00:00:00Z" {
		t.Errorf("SnapTime = %q, want RFC3339 conversion of the unix timestamp", got[0].SnapTime)
	}
	if got[1].SnapTime != "" {
		t.Errorf("zero SnapTime must stay empty, got %q", got[1].SnapTime)
	}
}

func TestNodeSnapshotEntryJSONShape(t *testing.T) {
	e := nodeSnapshotEntry{Name: "pre-upgrade", SnapTime: "2025-01-01T00:00:00Z"}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	s := string(data)
	for _, want := range []string{`"name":"pre-upgrade"`, `"snap_time":"2025-01-01T00:00:00Z"`} {
		if !strings.Contains(s, want) {
			t.Errorf("json output %q missing %q", s, want)
		}
	}
	for _, absent := range []string{"description", "parent"} {
		if strings.Contains(s, absent) {
			t.Errorf("json output %q must omit empty %q", s, absent)
		}
	}
}

func TestPrintNodeSnapshotList(t *testing.T) {
	entries := []nodeSnapshotEntry{
		{Name: "pre-upgrade", SnapTime: "2025-01-01T00:00:00Z", Description: "before upgrading"},
		{Name: "baseline"},
	}
	var buf bytes.Buffer
	if err := printNodeSnapshotList(&buf, entries); err != nil {
		t.Fatalf("printNodeSnapshotList: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "NAME") || !strings.Contains(out, "SNAPTIME") {
		t.Errorf("missing header: %q", out)
	}
	if !strings.Contains(out, "pre-upgrade") || !strings.Contains(out, "2025-01-01T00:00:00Z") {
		t.Errorf("missing pre-upgrade row: %q", out)
	}
	if !strings.Contains(out, "baseline") {
		t.Errorf("missing baseline row: %q", out)
	}
}

func TestPrintNodeSnapshotListEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := printNodeSnapshotList(&buf, nil); err != nil {
		t.Fatalf("printNodeSnapshotList: %v", err)
	}
	if got := buf.String(); got != "no snapshots found\n" {
		t.Errorf("printNodeSnapshotList(nil) = %q, want %q", got, "no snapshots found\n")
	}
}

func TestBuildSnapshotRunner_RequiresProxmoxProvider(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox = nil
	_, err := buildSnapshotRunner(context.Background(), cfg, false)
	var cfgErr *errtypes.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("want *errtypes.ConfigError, got %v (%T)", err, err)
	}
}
