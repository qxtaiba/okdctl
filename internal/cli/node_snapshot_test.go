package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/proxmox/hostssh"
)

// Rows with no stdin wired prove the gate never prompts on that path
// (promptForLine would hit the TTY guard and error).
func TestNodeSnapshotGate(t *testing.T) {
	cases := []struct {
		name          string
		verb          string
		twoStage, yes bool
		dryRun        bool
		confirm, warn string
		stdin         string // "" wires no stdin
		wantOK        bool
		wantUsageErr  bool
	}{
		{
			// --yes with no --confirm-cluster must fail closed regardless of twoStage.
			name: "yes skips prompt but still pairs confirm-cluster",
			verb: "node snapshot create", yes: true, wantUsageErr: true,
		},
		{
			name: "yes with matching confirm-cluster proceeds without prompt",
			verb: "node snapshot create", yes: true, confirm: "prod", warn: "warn", wantOK: true,
		},
		{
			name: "dry-run skips prompt even without yes",
			verb: "node snapshot rollback", twoStage: true, dryRun: true, warn: "warn", wantOK: true,
		},
		{
			name: "create single-stage y proceeds",
			verb: "node snapshot create", warn: "crash-consistent warning", stdin: "y\n", wantOK: true,
		},
		{
			name: "create single-stage plain n denies",
			verb: "node snapshot create", stdin: "n\n", wantOK: false,
		},
		{
			// The single-stage 'y' must NOT be enough for rollback's two-stage gate.
			name: "rollback two-stage requires typed name",
			verb: "node snapshot rollback", twoStage: true, stdin: "y\n", wantOK: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.stdin != "" {
				testStdinReader = strings.NewReader(tc.stdin)
				t.Cleanup(func() { testStdinReader = nil })
			}

			ok, err := nodeSnapshotGate(context.Background(), tc.verb, tc.twoStage, tc.yes, tc.dryRun, tc.confirm, "prod", tc.warn)
			if tc.wantUsageErr {
				var usageErr *errtypes.UsageError
				if !errors.As(err, &usageErr) {
					t.Fatalf("want *errtypes.UsageError, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("want nil error, got %v", err)
			}
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
		})
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
