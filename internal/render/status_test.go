package render

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/distribution/okd"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

// lipgloss.Width ignores zero-width ANSI codes, so a styled row's visual width
// matches an unstyled row's.
func TestStatusNodeTableLinesStylesNotReadyRows(t *testing.T) {
	nodes := []okd.NodeStatus{
		{Name: "worker-0", Role: nodetypes.RoleWorker, Ready: true},
		{Name: "worker-1", Role: nodetypes.RoleWorker, Ready: false},
	}
	lines := statusNodeTableLines(nodes)
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

func TestClusterStatusBoxWidthFitsTheRequestedWidth(t *testing.T) {
	st := &okd.ClusterStatus{
		Phase:        okd.PhaseRunning,
		APIReachable: true,
		Nodes: []okd.NodeStatus{
			{Name: "homelab-master0", Role: nodetypes.RoleMaster, Ready: true},
		},
	}

	const width = 64
	for _, line := range strings.Split(strings.TrimSpace(ClusterStatusBoxWidth(st, width)), "\n") {
		if got := lipgloss.Width(line); got > width {
			t.Fatalf("box row is %d cols wide at width %d: %q", got, width, line)
		}
	}
}
