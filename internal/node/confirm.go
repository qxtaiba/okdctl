package node

import (
	"context"
	"errors"

	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

// ErrDeclined reports that an operator declined a mutating node op at its
// confirmation gate. Ops return it — rather than a nil error — so a composing
// caller (compact) can never mistake a decline for success; standalone CLI
// callers map it back to a clean exit. errors.Is-comparable.
var ErrDeclined = errors.New("operation declined at confirmation")

// PlanNode is one node's line in an OpPlan: the terraform action queued against
// it plus the read-only guard findings that bear on the operator's decision.
// A non-nil Blocked means the plan would refuse this node (compact reports every
// worker's verdict; remove/resize abort before building a blocked plan).
type PlanNode struct {
	Name      string
	Role      nodetypes.NodeRole
	TFAddress string
	Action    terraform.PlanAction
	OSDs      []string
	Ingress   []string
	Blocked   error
}

// OpPlan is the read-only summary a node op hands to its confirm/preview hooks
// after guards and preflight pass and before any mutation, so the CLI can render
// an informed prompt or a dry-run box. Nodes are in execution order.
type OpPlan struct {
	Op                 Op
	Cluster            string
	Nodes              []PlanNode
	DrainTimeout       string
	MemoryMB           int
	CPU                int
	GrowMasterMemoryMB int
	IngressReplicas    int
}

// DestroysData reports whether the plan deletes a VM, taking its data disk with
// it — the irreversible case the confirmation box flags in amber.
func (p *OpPlan) DestroysData() bool {
	for i := range p.Nodes {
		if p.Nodes[i].Action == terraform.PlanActionDelete {
			return true
		}
	}
	return false
}

// ConfirmFunc gates a mutating node op. It runs after the op's read-only guards
// and preflight pass and before any mutation; returning (false, nil) aborts and
// the op reports ErrDeclined, while a non-nil error propagates. It never runs in
// dry-run, and it is invoked with no progress span open so an interactive prompt
// never fights a spinner.
type ConfirmFunc func(ctx context.Context, plan *OpPlan) (bool, error)

// PreviewFunc renders a dry-run summary of plan. It never gates and never runs
// outside dry-run.
type PreviewFunc func(plan *OpPlan)

// confirm runs the op's consent gate. Once a composing op (compact) has taken
// top-level consent it sets preConsented, so inner RemoveWorker/Resize calls
// run under that single grant instead of re-prompting mid-teardown: a mistyped
// name or Ctrl-C at an inner prompt must not abort a half-executed sequence,
// and — with the gate suppressed — an inner decline is impossible by
// construction (ErrDeclined is the belt to that suspenders).
func (r *Runner) confirm(ctx context.Context, plan *OpPlan) (bool, error) {
	if r.preConsented {
		return true, nil
	}
	if r.Confirm == nil {
		return true, nil
	}
	return r.Confirm(ctx, plan)
}

func (r *Runner) preview(plan *OpPlan) {
	if r.Preview != nil {
		r.Preview(plan)
	}
}
