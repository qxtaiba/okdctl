package cli

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/distribution/okd"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

// TestNodeStatusTableLinesAlignsColumnsWithLongNames covers only Ready nodes
// (unstyled rows) so raw byte offsets are a valid alignment proxy — a
// NotReady row's ANSI wrapping is covered separately since it shifts byte
// offsets without shifting visual columns.
func TestNodeStatusTableLinesAlignsColumnsWithLongNames(t *testing.T) {
	nodes := []okd.NodeStatus{
		{Name: "m0", Role: nodetypes.RoleMaster, Ready: true},
		{Name: "worker-extraordinarily-long-hostname-12", Role: nodetypes.RoleWorker, Ready: true},
	}
	lines := nodeStatusTableLines(nodes)
	if len(lines) != 3 {
		t.Fatalf("len(lines) = %d, want 3 (header + 2 rows)", len(lines))
	}
	header, row0, row1 := lines[0], lines[1], lines[2]

	headers := []string{"NAME", "ROLE", "READY"}
	colStarts := make([]int, len(headers))
	for i, h := range headers {
		idx := strings.Index(header, h)
		if idx == -1 {
			t.Fatalf("header missing %q: %q", h, header)
		}
		colStarts[i] = idx
	}
	col := func(line string, i int) string {
		start := min(colStarts[i], len(line))
		end := len(line)
		if i+1 < len(colStarts) {
			end = min(colStarts[i+1], len(line))
		}
		return strings.TrimSpace(line[start:end])
	}

	if got := col(row0, 1); got != "master" {
		t.Errorf("row0 ROLE column = %q, want %q:\n%s", got, "master", strings.Join(lines, "\n"))
	}
	if got := col(row1, 1); got != "worker" {
		t.Errorf("row1 ROLE column = %q, want %q:\n%s", got, "worker", strings.Join(lines, "\n"))
	}
	if got := col(row0, 2); got != "yes" {
		t.Errorf("row0 READY column = %q, want %q", got, "yes")
	}
}

// TestNodeStatusTableLinesStylesNotReadyRows asserts NotReady rows are
// wrapped in tui.ErrorStyle while keeping the same visual width (lipgloss.Width
// ignores the zero-width ANSI codes) as an equivalent unstyled row, so the
// styling never perturbs the box's column padding.
func TestNodeStatusTableLinesStylesNotReadyRows(t *testing.T) {
	nodes := []okd.NodeStatus{
		{Name: "worker-0", Role: nodetypes.RoleWorker, Ready: true},
		{Name: "worker-1", Role: nodetypes.RoleWorker, Ready: false},
	}
	lines := nodeStatusTableLines(nodes)
	readyRow, notReadyRow := lines[1], lines[2]

	if strings.Contains(readyRow, "\x1b[") {
		t.Errorf("ready row must not carry ANSI styling: %q", readyRow)
	}
	if !strings.Contains(notReadyRow, "\x1b[") {
		t.Errorf("not-ready row must carry ANSI error styling: %q", notReadyRow)
	}
	if !strings.Contains(notReadyRow, "no") {
		t.Errorf("not-ready row must still render its READY=no cell: %q", notReadyRow)
	}
	if lipgloss.Width(readyRow) != lipgloss.Width(notReadyRow) {
		t.Errorf("styled row visual width = %d, want %d (same column layout as the unstyled row)",
			lipgloss.Width(notReadyRow), lipgloss.Width(readyRow))
	}
}

func TestPrintClusterStatusIncludesTableAndFooterCounts(t *testing.T) {
	st := &okd.ClusterStatus{
		Phase:        okd.PhaseDegraded,
		APIReachable: true,
		Nodes: []okd.NodeStatus{
			{Name: "master-0", Role: nodetypes.RoleMaster, Ready: true},
			{Name: "worker-0", Role: nodetypes.RoleWorker, Ready: false},
		},
		DegradedOperators: 1,
	}

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)

	if err := printClusterStatus(cmd, st); err != nil {
		t.Fatalf("printClusterStatus: %v", err)
	}
	out := buf.String()

	for _, want := range []string{"master-0", "worker-0", "masters", "workers", "total", strconv.Itoa(len(st.Nodes))} {
		if !strings.Contains(out, want) {
			t.Errorf("status output missing %q:\n%s", want, out)
		}
	}
}

func TestPrintClusterStatusNoNodesReported(t *testing.T) {
	st := &okd.ClusterStatus{Phase: okd.PhaseUnknown}

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)

	if err := printClusterStatus(cmd, st); err != nil {
		t.Fatalf("printClusterStatus: %v", err)
	}
	if !strings.Contains(buf.String(), "no nodes reported") {
		t.Errorf("status output missing empty-nodes message:\n%s", buf.String())
	}
}
