package render

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/tui"
)

// IrreversibleWarning is the amber wording shared by the CLI and wizard for
// a destructive node op that also destroys a data disk.
const IrreversibleWarning = "destroys the listed VM(s) and their data disk; removed data cannot be recovered"

// colRole is the role column heading shared by every per-node table.
const colRole = "ROLE"

// nodeRoleStateHeaders labels the per-node table for stop/start plans and
// for the completion box, none of which carry a terraform address.
var nodeRoleStateHeaders = []string{"NODE", colRole, "STATE"}

// nodeRoleAddressActionHeaders labels the per-node table for every other op,
// where the terraform address and its queued action are worth showing.
var nodeRoleAddressActionHeaders = []string{"NODE", colRole, "ADDRESS", "ACTION"}

// NodeOpConfirm renders the preview before a destructive node op; it prints even under --yes.
func NodeOpConfirm(plan *node.OpPlan) string {
	sb := NewBuilder()
	sb.WriteString("\n")
	sb.WriteString("  " + tui.HighlightStyle.Render(opHeadline(plan.Op)) + "\n")
	sb.Newline()

	nodeOpDetails(sb, plan)

	if plan.DestroysData() {
		sb.WriteString("  " + tui.WarningStyle.Render("irreversible: "+IrreversibleWarning) + "\n")
		sb.Newline()
	}

	return "\n" + tui.BoxedSectionCompact(sb.String(), opTitle(plan.Op), tui.DefaultBoxWidth) + "\n"
}

// NodeOpDryRun renders the ordered-operations summary for a node-op dry-run.
func NodeOpDryRun(plan *node.OpPlan) string {
	sb := NewBuilder()
	sb.WriteString("\n")
	sb.WriteString("  " + tui.WarningStyle.Render("dry-run — no changes made") + "\n")
	sb.Newline()

	nodeOpDetails(sb, plan)

	sb.Section("next steps")
	sb.WriteString("    re-run without " + tui.CodeInlineStyle.Render("--dry-run") + " to execute\n")
	sb.Newline()

	return "\n" + tui.BoxedSectionCompact(sb.String(), opTitle(plan.Op), tui.DefaultBoxWidth) + "\n"
}

// NodeOpComplete renders the completion box after a node op succeeds, listing
// the affected nodes, elapsed time, and any operator-owned follow-up.
func NodeOpComplete(plan *node.OpPlan, elapsed time.Duration) string {
	return NodeOpCompleteWidth(plan, elapsed, tui.DefaultBoxWidth)
}

// NodeOpCompleteWidth renders NodeOpComplete sized to fit inside a box of
// the given width, for callers that must fit a narrower viewport.
func NodeOpCompleteWidth(plan *node.OpPlan, elapsed time.Duration, width int) string {
	sb := NewBuilderWidth(width)
	sb.WriteString("\n")
	sb.WriteString("  " + tui.CompletionSuccess(opComplete(plan.Op)) + "\n")
	sb.Newline()
	sb.KV("cluster", plan.Cluster)
	sb.KV("elapsed", elapsed.Truncate(time.Second).String())
	sb.Newline()

	sb.Section("nodes")
	rows := make([][]string, len(plan.Nodes))
	for i := range plan.Nodes {
		n := &plan.Nodes[i]
		verb := nodeActionVerb(n.Action)
		if plan.Op == node.OpStop || plan.Op == node.OpStart {
			verb = nodePowerCompleteVerb(plan.Op)
		}
		rows[i] = []string{shortHost(n.Name), string(n.Role), verb}
	}
	sb.Table(nodeRoleStateHeaders, rows, tui.TableOptions{})
	sb.Newline()

	if steps := NodeOpNextSteps(plan); len(steps) > 0 {
		sb.Section("next steps")
		// avail is the box's real inner content width (tui.BoxInnerWidth,
		// the same figure BoxedSectionCompact derives below) less the
		// 4-column margin every line here carries, matching the indent
		// tableIndent uses for other Section-body content. A narrow
		// terminal can still make an authored next-step line too long for
		// one row; wrap it rather than truncate, so a follow-up instruction
		// (e.g. "verify with 'okdctl node list'") is never cut mid-word.
		avail := tui.BoxInnerWidth(width) - 4
		for _, s := range steps {
			for _, line := range tui.WrapLines(s, avail) {
				sb.WriteString("    " + line + "\n")
			}
		}
		sb.Newline()
	}

	return "\n" + tui.BoxedSectionCompact(sb.String(), opTitle(plan.Op), width) + "\n"
}

// NodeOpRecapLines renders the short plain-text recap printed to stdout
// after a successful lifecycle-wizard op — the durable record the AltScreen
// otherwise clears on exit — reusing opComplete and NodeOpNextSteps
// verbatim so wording never drifts from the done card's own.
func NodeOpRecapLines(plan *node.OpPlan, elapsed time.Duration) []string {
	names := make([]string, len(plan.Nodes))
	for i := range plan.Nodes {
		names[i] = shortHost(plan.Nodes[i].Name)
	}
	lines := []string{
		fmt.Sprintf("%s · %s · %s", opComplete(plan.Op), strings.Join(names, ", "), elapsed.Truncate(time.Second)),
	}
	return append(lines, NodeOpNextSteps(plan)...)
}

// nodeOpDetails writes the shared header + per-node section for the confirm and dry-run boxes.
func nodeOpDetails(sb *Builder, plan *node.OpPlan) {
	sb.KV("cluster", plan.Cluster)
	sb.KV("operation", string(plan.Op))
	if plan.DrainTimeout != "" {
		sb.KV("drain timeout", plan.DrainTimeout)
	}
	if plan.Op == node.OpResize {
		if plan.MemoryMB > 0 {
			sb.KV("target memory", fmt.Sprintf("%d MiB", plan.MemoryMB))
		}
		if plan.CPU > 0 {
			sb.KV("target cpu", fmt.Sprintf("%d vCPU", plan.CPU))
		}
		if plan.OSDiskGB > 0 {
			sb.KV("target os disk", fmt.Sprintf("%d GiB", plan.OSDiskGB))
		}
		sb.KV("disruption", ResizeDisruption(plan.ResizeMode))
	}
	if plan.Op == node.OpAdd {
		sb.KV("ignition server", "revived for the join window, then torn down")
	}
	sb.Newline()

	sb.Section("nodes")
	if plan.Op == node.OpStop || plan.Op == node.OpStart {
		rows := make([][]string, len(plan.Nodes))
		for i := range plan.Nodes {
			n := &plan.Nodes[i]
			rows[i] = []string{shortHost(n.Name), string(n.Role), nodePowerPlanVerb(plan.Op)}
		}
		sb.Table(nodeRoleStateHeaders, rows, tui.TableOptions{})
	} else {
		rows := make([][]string, len(plan.Nodes))
		for i := range plan.Nodes {
			n := &plan.Nodes[i]
			rows[i] = []string{shortHost(n.Name), string(n.Role), n.TFAddress, string(n.Action)}
		}
		sb.Table(nodeRoleAddressActionHeaders, rows, tui.TableOptions{MaxColWidth: 40})
	}
	for i := range plan.Nodes {
		n := &plan.Nodes[i]
		if len(n.OSDs) > 0 {
			sb.SubKV(shortHost(n.Name)+" storage", fmt.Sprintf("%d rook-ceph OSD(s) — data disk destroyed", len(n.OSDs)))
		}
		if len(n.Ingress) > 0 {
			sb.SubKV(shortHost(n.Name)+" ingress", fmt.Sprintf("%d router pod(s) here", len(n.Ingress)))
		}
		if n.Blocked != nil {
			// n.Blocked can wrap a terraform-plan error carrying raw
			// subprocess output, unlike the rest of this function's
			// okdctl-composed strings.
			sb.SubKV(shortHost(n.Name)+" blocked", tui.SanitizeTerminalEscapes(n.Blocked.Error()))
		}
	}
	sb.Newline()
}

// shortHost returns name's first DNS label, or name unchanged when it's an IP address or carries no dot.
func shortHost(name string) string {
	if net.ParseIP(name) != nil {
		return name
	}
	if i := strings.IndexByte(name, '.'); i >= 0 {
		return name[:i]
	}
	return name
}

// NodeOpNextSteps returns the operator follow-ups for a completed op; shared by
// the CLI and wizard so wording never drifts.
func NodeOpNextSteps(plan *node.OpPlan) []string {
	switch plan.Op {
	case node.OpAdd:
		return []string{
			"if haproxy fronts this cluster, add 'server' lines for the new worker(s) to",
			"  /etc/haproxy/haproxy.cfg, validate with 'haproxy -c -f ...', then restart it",
			"verify the new node(s) joined with 'okdctl node list' or 'okdctl status'",
		}
	case node.OpRemove:
		return []string{
			"if haproxy fronts this cluster, drop the removed worker 'server' lines from",
			"  /etc/haproxy/haproxy.cfg, validate with 'haproxy -c -f ...', then restart it",
			"verify the cluster with 'okdctl status'",
		}
	case node.OpResize:
		if plan.ResizeMode == node.ResizeLiveDisk {
			return []string{
				"the os disk was grown live in-guest; no node was power-cycled",
				"  verify with 'okdctl node list' or 'oc debug node/<name> -- df -h'",
			}
		}
		return []string{
			"each resized node was power-cycled to realize the change; verify with",
			"  'okdctl node list' or 'oc debug node/<name> -- free -m'",
		}
	case node.OpStop:
		return []string{
			"the cluster is powered off and nothing will respond until it restarts",
			"restart it with 'okdctl cluster start'",
		}
	case node.OpStart:
		return []string{
			"verify the cluster with 'okdctl status'",
		}
	default:
		return nil
	}
}

// nodePowerPlanVerb replaces the [action] bracket for stop/start ops, which
// carry no terraform address.
func nodePowerPlanVerb(op node.Op) string {
	if op == node.OpStart {
		return "powered on"
	}
	return "shut down"
}

func nodePowerCompleteVerb(op node.Op) string {
	if op == node.OpStart {
		return "started"
	}
	return "stopped"
}

func nodeActionVerb(a terraform.PlanAction) string {
	switch a {
	case terraform.PlanActionCreate:
		return "added"
	case terraform.PlanActionDelete:
		return "removed"
	case terraform.PlanActionUpdate:
		return "resized"
	default:
		return string(a)
	}
}

func opHeadline(op node.Op) string {
	switch op {
	case node.OpAdd:
		return "confirm node add"
	case node.OpRemove:
		return "confirm worker removal"
	case node.OpResize:
		return "confirm node resize"
	case node.OpStop:
		return "confirm cluster stop"
	case node.OpStart:
		return "confirm cluster start"
	default:
		return "confirm node operation"
	}
}

func opTitle(op node.Op) string {
	switch op {
	case node.OpAdd:
		return "node add"
	case node.OpRemove:
		return "node remove"
	case node.OpResize:
		return "node resize"
	case node.OpStop:
		return "cluster stop"
	case node.OpStart:
		return "cluster start"
	default:
		return "node op"
	}
}

func opComplete(op node.Op) string {
	switch op {
	case node.OpAdd:
		return "worker(s) added"
	case node.OpRemove:
		return "worker removed"
	case node.OpResize:
		return "resize complete"
	case node.OpStop:
		return "cluster stopped"
	case node.OpStart:
		return "cluster started"
	default:
		return "node operation complete"
	}
}

// ResizeDisruption describes the approved resize behavior for CLI and wizard previews.
func ResizeDisruption(mode node.ResizeMode) string {
	switch mode {
	case node.ResizeLiveDisk:
		return "live resize — no drain, no power-cycle"
	case node.ResizeUndrainedRestart:
		return "power-cycle without drain (pods restart in place)"
	default:
		return "each node is drained, then hard power-cycled (stop→start) to realize the change"
	}
}
