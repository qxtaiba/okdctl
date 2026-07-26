package lifecycle

import (
	"testing"

	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/node"
)

func TestPlansEquivalent(t *testing.T) {
	base := func() *node.OpPlan {
		return &node.OpPlan{Op: node.OpResize, Cluster: "homelab", Nodes: []node.PlanNode{
			{Name: "master0", TFAddress: "a.b.master[0]", Action: terraform.PlanActionUpdate},
		}}
	}
	if !PlansEquivalent(base(), base()) {
		t.Error("identical plans must be equivalent")
	}
	mutations := []func(*node.OpPlan){
		func(p *node.OpPlan) { p.Op = node.OpRemove },
		func(p *node.OpPlan) { p.Cluster = "other" },
		func(p *node.OpPlan) { p.Nodes = p.Nodes[:0] },
		func(p *node.OpPlan) { p.Nodes[0].Action = terraform.PlanActionDelete },
		func(p *node.OpPlan) { p.Nodes[0].TFAddress = "a.b.master[1]" },
		func(p *node.OpPlan) { p.Nodes[0].Name = "master1" },
	}
	for i, mutate := range mutations {
		p := base()
		mutate(p)
		if PlansEquivalent(base(), p) {
			t.Errorf("mutation %d must break equivalence", i)
		}
	}
	if PlansEquivalent(nil, base()) || PlansEquivalent(base(), nil) {
		t.Error("nil plans are never equivalent")
	}
}
