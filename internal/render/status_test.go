package render

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/distribution/okd"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

// lipgloss.Width ignores zero-width ANSI codes, so a styled row's visual width
// matches an unstyled row's.
func TestStatusNodeTableLinesStylesNotReadyRows(t *testing.T) {
	nodes := []okd.NodeStatus{
		{Name: "worker-0", Role: nodetypes.RoleWorker, Ready: true},
		{Name: "worker-1", Role: nodetypes.RoleWorker, Ready: false},
	}
	lines := statusNodeTableLines(nodes)
	readyRow, notReadyRow := lines[2], lines[3]

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

// TestStatusNodeTableLinesGroupsByRole pins the role-grouped layout: dim
// "masters (n)"/"workers (n)" title lines carry the role, so the table
// itself drops the redundant ROLE column.
func TestStatusNodeTableLinesGroupsByRole(t *testing.T) {
	nodes := []okd.NodeStatus{
		{Name: "worker-0", Role: nodetypes.RoleWorker, Ready: true},
		{Name: "master-0", Role: nodetypes.RoleMaster, Ready: true},
		{Name: "master-1", Role: nodetypes.RoleMaster, Ready: true},
	}
	lines := statusNodeTableLines(nodes)

	var plain []string
	for _, l := range lines {
		plain = append(plain, strings.TrimRight(tuitest.StripANSI(l), " "))
	}
	want := []string{"NAME      READY", "masters (2)", "master-0  yes", "master-1  yes", "workers (1)", "worker-0  yes"}
	if len(plain) != len(want) {
		t.Fatalf("lines = %q, want %q", plain, want)
	}
	for i := range want {
		if plain[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, plain[i], want[i])
		}
	}
	if strings.Contains(strings.Join(plain, "\n"), "ROLE") {
		t.Error("ROLE column must be absorbed by the group titles")
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
