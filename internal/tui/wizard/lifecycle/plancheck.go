package lifecycle

import "github.com/qxtaiba/okdctl/internal/node"

// PlansEquivalent reports whether two OpPlans describe the same mutation —
// the staleness check run by the executing Runner's ConfirmFunc against
// the plan the operator approved on the preview screen.
func PlansEquivalent(a, b *node.OpPlan) bool {
	if a == nil || b == nil {
		return false
	}
	if a.Op != b.Op || a.Cluster != b.Cluster || len(a.Nodes) != len(b.Nodes) {
		return false
	}
	for i := range a.Nodes {
		if a.Nodes[i].Name != b.Nodes[i].Name ||
			a.Nodes[i].TFAddress != b.Nodes[i].TFAddress ||
			a.Nodes[i].Action != b.Nodes[i].Action {
			return false
		}
	}
	return true
}
