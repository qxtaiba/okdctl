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

// lipgloss.Width ignores zero-width ANSI codes, so a styled row's visual width
// matches an unstyled row's.
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
