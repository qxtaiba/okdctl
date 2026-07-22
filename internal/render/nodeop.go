package render

import (
	"fmt"
	"time"

	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/tui"
)

// NodeOpConfirm renders the informed preview shown before a destructive node op
// runs: the target nodes, their terraform addresses and actions, the read-only
// guard verdicts, and — when the plan destroys a VM — an amber irreversible
// warning naming the VM and its data disk. It is printed whether or not the
// operator is prompted, so --yes runs still surface what is about to happen.
func NodeOpConfirm(plan *node.OpPlan) string {
	sb := NewBuilder()
	sb.WriteString("\n")
	sb.WriteString("  " + tui.HighlightStyle.Render(opHeadline(plan.Op)) + "\n")
	sb.Newline()

	nodeOpDetails(sb, plan)

	if plan.DestroysData() {
		sb.WriteString("  " + tui.WarningStyle.Render(
			"irreversible: destroys the listed VM(s) and their data disk; removed data cannot be recovered") + "\n")
		sb.Newline()
	}

	return "\n" + tui.BoxedSectionCompact(sb.String(), opTitle(plan.Op), tui.DefaultBoxWidth) + "\n"
}

// NodeOpDryRun renders the ordered-operations summary for a dry-run of a node
// op, mirroring the deploy-family dry-run box: no mutation, just the plan.
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

// NodeOpComplete renders the completion box shown after a node op succeeds,
// listing the nodes acted on, the elapsed time, and the operation-specific
// follow-up the operator still owns (HAProxy backend refresh, verification).
func NodeOpComplete(plan *node.OpPlan, elapsed time.Duration) string {
	sb := NewBuilder()
	sb.WriteString("\n")
	sb.WriteString("  " + tui.CompletionSuccess(opComplete(plan.Op)) + "\n")
	sb.Newline()
	sb.KV("cluster", plan.Cluster)
	sb.KV("elapsed", elapsed.Truncate(time.Second).String())
	sb.Newline()

	sb.Section("nodes")
	for i := range plan.Nodes {
		n := &plan.Nodes[i]
		verb := nodeActionVerb(n.Action)
		if plan.Op == node.OpStop || plan.Op == node.OpStart {
			verb = nodePowerCompleteVerb(plan.Op)
		}
		sb.KV(n.Name, fmt.Sprintf("%s  %s", n.Role, verb))
	}
	sb.Newline()

	if steps := nodeOpNextSteps(plan); len(steps) > 0 {
		sb.Section("next steps")
		for _, s := range steps {
			sb.WriteString("    " + s + "\n")
		}
		sb.Newline()
	}

	return "\n" + tui.BoxedSectionCompact(sb.String(), opTitle(plan.Op), tui.DefaultBoxWidth) + "\n"
}

// nodeOpDetails writes the shared header + per-node section used by the confirm
// and dry-run boxes.
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
		sb.KV("disruption", "each node is drained, then hard power-cycled (stop→start) to realize the change")
	}
	if plan.GrowMasterMemoryMB > 0 {
		sb.KV("grow masters to", fmt.Sprintf("%d MiB", plan.GrowMasterMemoryMB))
	}
	sb.Newline()

	sb.Section("nodes")
	for i := range plan.Nodes {
		n := &plan.Nodes[i]
		if plan.Op == node.OpStop || plan.Op == node.OpStart {
			sb.KV(n.Name, fmt.Sprintf("%s  %s", n.Role, nodePowerPlanVerb(plan.Op)))
		} else {
			sb.KV(n.Name, fmt.Sprintf("%s  %s  [%s]", n.Role, n.TFAddress, n.Action))
		}
		if len(n.OSDs) > 0 {
			sb.KV("  storage", fmt.Sprintf("%d rook-ceph OSD(s) — data disk destroyed", len(n.OSDs)))
		}
		if len(n.Ingress) > 0 {
			sb.KV("  ingress", fmt.Sprintf("%d router pod(s) here", len(n.Ingress)))
		}
		if n.Blocked != nil {
			sb.KV("  blocked", n.Blocked.Error())
		}
	}
	sb.Newline()
}

func nodeOpNextSteps(plan *node.OpPlan) []string {
	switch plan.Op {
	case node.OpRemove, node.OpCompact:
		return []string{
			"if haproxy fronts this cluster, drop the removed worker 'server' lines from",
			"  /etc/haproxy/haproxy.cfg, validate with 'haproxy -c -f ...', then restart it",
			"verify the cluster with 'okdctl status'",
		}
	case node.OpResize:
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

// nodePowerPlanVerb names the planned power action for a stop/start node line
// in the confirm and dry-run boxes; these ops carry no terraform address, so
// this replaces the [action] bracket used by tf-mutating ops.
func nodePowerPlanVerb(op node.Op) string {
	if op == node.OpStart {
		return "powered on"
	}
	return "shut down"
}

// nodePowerCompleteVerb names the completed power action for a stop/start node
// line in the completion box.
func nodePowerCompleteVerb(op node.Op) string {
	if op == node.OpStart {
		return "started"
	}
	return "stopped"
}

func nodeActionVerb(a terraform.PlanAction) string {
	switch a {
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
	case node.OpRemove:
		return "confirm worker removal"
	case node.OpCompact:
		return "confirm cluster compaction"
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
	case node.OpRemove:
		return "node remove"
	case node.OpCompact:
		return "cluster compact"
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
	case node.OpRemove:
		return "worker removed"
	case node.OpCompact:
		return "cluster compacted"
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
