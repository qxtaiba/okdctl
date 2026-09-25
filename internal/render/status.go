package render

import (
	"strconv"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/distribution/okd"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/tui"
)

// statusNodeHeader is the node table's name column heading.
const statusNodeHeader = "NAME"

// ClusterStatusBox renders st as the boxed cluster-status summary at the
// default box width, the form okdctl status prints.
func ClusterStatusBox(st *okd.ClusterStatus) string {
	return ClusterStatusBoxWidth(st, tui.DefaultBoxWidth)
}

// ClusterStatusBoxWidth renders st as the boxed cluster-status summary at width
// columns, so an in-wizard viewport can draw the same box its narrower frame
// has room for.
func ClusterStatusBoxWidth(st *okd.ClusterStatus, width int) string {
	sb := NewBuilderWidth(width)
	sb.WriteString("\n")

	sb.Section("cluster")
	sb.KV("phase", string(st.Phase))
	sb.Newline()

	sb.Section("api")
	if st.APIReachable {
		sb.KV("reachable", "yes")
	} else {
		sb.KV("reachable", "no (oc get --raw /healthz failed)")
	}
	sb.Newline()

	writeStatusNodes(sb, st.Nodes)

	sb.Section("cluster operators")
	if st.DegradedOperators == 0 {
		sb.KV("degraded", "0 (all healthy)")
	} else {
		sb.KV("degraded", strconv.Itoa(st.DegradedOperators))
	}
	sb.Newline()

	if len(st.Addons) > 0 {
		sb.Section("addons")
		for _, a := range st.Addons {
			sb.KV(a.Name, a.Label())
		}
		sb.Newline()
	}

	return "\n" + tui.BoxedSectionCompact(sb.String(), "cluster status", width) + "\n"
}

// writeStatusNodes writes the nodes section: the per-node table plus the
// role tallies, or an empty state when the cluster reported none.
func writeStatusNodes(sb *Builder, nodes []okd.NodeStatus) {
	masters, workers := 0, 0
	for _, n := range nodes {
		switch n.Role {
		case nodetypes.RoleMaster:
			masters++
		case nodetypes.RoleWorker:
			workers++
		}
	}

	sb.Section("nodes")
	if len(nodes) == 0 {
		sb.WriteString("    " + tui.EmptyState("no nodes reported", "deploy a cluster with 'okdctl deploy'") + "\n")
		sb.Newline()
	} else {
		for _, line := range statusNodeTableLines(nodes) {
			sb.WriteString("    " + line + "\n")
		}
		sb.Newline()
	}
	sb.KV("masters", strconv.Itoa(masters))
	sb.KV("workers", strconv.Itoa(workers))
	sb.KV("total", strconv.Itoa(len(nodes)))
	sb.Newline()
}

// statusNodeTableLines renders via tui.Table; padding is computed on plain text
// so a styled row's zero-width escapes never shift a column.
func statusNodeTableLines(nodes []okd.NodeStatus) []string {
	rows := make([][]string, 0, len(nodes))
	for _, n := range nodes {
		rows = append(rows, []string{n.Name, string(n.Role), statusYesNo(n.Ready)})
	}
	return tui.Table([]string{statusNodeHeader, colRole, "READY"}, rows, tui.TableOptions{
		RowStyle: func(i int) (lipgloss.Style, bool) {
			if !nodes[i].Ready {
				return tui.ErrorStyle, true
			}
			return lipgloss.Style{}, false
		},
	})
}

// statusYesNo renders a readiness bool as the table's yes/no cell.
func statusYesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
