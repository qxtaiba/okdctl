package node

import (
	"context"
	"errors"

	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

// ErrDeclined reports an operator's decline at the confirmation gate, never nil
// so a composing caller can't mistake it for success.
var ErrDeclined = errors.New("operation declined at confirmation")

// PlanNode is one node's line in an OpPlan: its queued terraform action plus guard findings.
// A non-nil Blocked means the plan would refuse this node.
type PlanNode struct {
	Name      string
	Role      nodetypes.NodeRole
	TFAddress string
	Action    terraform.PlanAction
	OSDs      []string
	Ingress   []string
	Blocked   error
}

// OpPlan is the read-only summary a node op hands to confirm/preview hooks before any mutation.
// Nodes are in execution order.
type OpPlan struct {
	Op                 Op
	Cluster            string
	Nodes              []PlanNode
	DrainTimeout       string
	MemoryMB           int
	CPU                int
	OSDiskGB           int
	GrowMasterMemoryMB int
	IngressReplicas    int
}

// DestroysData reports whether the plan deletes a VM (and its data disk) — the
// case the confirmation box flags in amber.
func (p *OpPlan) DestroysData() bool {
	for i := range p.Nodes {
		if p.Nodes[i].Action == terraform.PlanActionDelete {
			return true
		}
	}
	return false
}

// ConfirmFunc gates a mutating op between preflight and the first mutation:
// (false, nil) aborts with ErrDeclined, a non-nil error propagates. It never
// runs in dry-run or with a progress span open.
type ConfirmFunc func(ctx context.Context, plan *OpPlan) (bool, error)

// PreviewFunc renders a dry-run summary of plan; it never gates and never runs outside dry-run.
type PreviewFunc func(plan *OpPlan)

// confirm suppresses the gate when preConsented (set by a composing op like
// compact), avoiding re-prompts mid-teardown.
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

// confirmOrDecline returns ErrDeclined (logging cancelMsg) on decline or a
// confirm error; callers own their dry-run/resume guards around this call.
func (r *Runner) confirmOrDecline(ctx context.Context, plan *OpPlan, cancelMsg string, kv ...any) error {
	proceed, err := r.confirm(ctx, plan)
	if err != nil {
		return err
	}
	if !proceed {
		r.Log.Info(cancelMsg, kv...)
		return ErrDeclined
	}
	return nil
}
