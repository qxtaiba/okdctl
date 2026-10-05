package render

import (
	"fmt"
	"slices"
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

// writeStatusNodes writes the nodes section: the role-grouped per-node
// table (group titles carry the per-role tallies) plus the total, or an
// empty state when the cluster reported none.
func writeStatusNodes(sb *Builder, nodes []okd.NodeStatus) {
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
	sb.KV("total", strconv.Itoa(len(nodes)))
	sb.Newline()
}

// statusNodeTableLines renders the role-grouped node table: dim
// "masters (n)"/"workers (n)" title lines absorb the ROLE column, and
// padding is computed on plain text so a styled row's zero-width escapes
// never shift a column.
func statusNodeTableLines(nodes []okd.NodeStatus) []string {
	groups, flat := statusNodeGroups(nodes)
	return tui.ColumnTable(
		[]tui.Column{{Header: statusNodeHeader}, {Header: "READY"}},
		groups,
		tui.TableOptions{RowStyle: func(i int) (lipgloss.Style, bool) {
			if !flat[i].Ready {
				return tui.ErrorStyle, true
			}
			return lipgloss.Style{}, false
		}},
	)
}

// statusNodeGroups buckets nodes into role groups in master, worker, then
// first-seen order, keeping input order within each; flat mirrors the render
// order so the table's continuous row index maps back to its node.
func statusNodeGroups(nodes []okd.NodeStatus) (groups []tui.RowGroup, flat []okd.NodeStatus) {
	order := []nodetypes.NodeRole{nodetypes.RoleMaster, nodetypes.RoleWorker}
	for _, n := range nodes {
		if !slices.Contains(order, n.Role) {
			order = append(order, n.Role)
		}
	}
	for _, role := range order {
		var members []okd.NodeStatus
		for _, n := range nodes {
			if n.Role == role {
				members = append(members, n)
			}
		}
		if len(members) == 0 {
			continue
		}
		rows := make([][]string, len(members))
		for i, n := range members {
			rows[i] = []string{n.Name, statusYesNo(n.Ready)}
		}
		groups = append(groups, tui.RowGroup{
			Title: fmt.Sprintf("%ss (%d)", role, len(members)),
			Rows:  rows,
		})
		flat = append(flat, members...)
	}
	return groups, flat
}

// statusYesNo renders a readiness bool as the table's yes/no cell.
func statusYesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
