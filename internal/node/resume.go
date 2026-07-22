package node

import (
	"fmt"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/errtypes"
)

// OpMatch reports whether an on-disk marker belongs to the op the caller is
// about to run. beginOp resumes only when the marker's Op matches and OpMatch
// returns true, so each op supplies a predicate keyed on its own target
// (node name, role, …). Pure: it must not touch the cluster or terraform.
type OpMatch func(m *OpMarker) bool

// stepOrder totally orders the mutating-step vocabulary so shouldRunStep can
// decide which steps a resume replays. A resumed op re-enters the sequence at
// its recorded step; every step at or after it re-runs (steps are idempotent),
// every strictly-earlier step is skipped.
//
// shouldRunStep only ever compares two steps recorded by the same op's own
// beginOp resume (a foreign marker never reaches it — see beginOp), so the
// map only needs to stay internally monotonic within each op's own step
// subset; it is not a single shared timeline across ops. Node add's sequence
// reuses StepTFApply's slot (2) for its worker_count apply, so its own steps
// are numbered around that fixed point rather than renumbering it.
var stepOrder = map[Step]int{
	StepCordon:     0,
	StepDrain:      1,
	StepTFApply:    2,
	StepPowerCycle: 3,
	StepDeleteK8s:  4,
	StepUncordon:   5,

	StepBuildISO:     -3,
	StepUploadISO:    -2,
	StepIgnitionUp:   -1,
	StepWaitJoin:     6,
	StepIgnitionDown: 7,
}

// shouldRunStep reports whether step runs given the resume point from. from ""
// means "not resuming" — a fresh op runs every step. Otherwise from is the step
// the interrupted run was about to perform, so it and everything after it
// re-run while strictly-earlier steps are skipped as already-done.
func shouldRunStep(step, from Step) bool {
	if from == "" {
		return true
	}
	return stepOrder[step] >= stepOrder[from]
}

// beginOp is the first call in every mutating op, ahead of guards, validation,
// and the confirm gate. It classifies the on-disk marker into one of four
// outcomes and is strictly READ-ONLY (only ReadOpMarker's file read) so it
// cannot break the dry-run zero-mutation contract:
//
//   - no marker: (nil, nil) — run normally.
//   - marker's Op matches and match() is true: (marker, nil) — resume; the
//     caller skips guards/confirm and gates each step via shouldRunStep.
//   - a foreign marker (different op, or the same op but a non-matching
//     target) with ack false: a ConfigError naming the stranded op/step/node,
//     pointing at --acknowledge-interrupted-op.
//   - a foreign marker with ack true: warn and (nil, nil) — the op proceeds
//     fresh and its first markStep overwrites the stale marker.
func (r *Runner) beginOp(op Op, match OpMatch, ack bool) (*OpMarker, error) {
	marker, err := ReadOpMarker(r.WorkDir, r.Cfg.Cluster.Name)
	if err != nil {
		return nil, err
	}
	if marker == nil {
		return nil, nil
	}
	if marker.Op == op && match(marker) {
		r.Log.Info("node: resuming interrupted op",
			"op", string(marker.Op), "node", marker.Target, "step", string(marker.Step))
		return marker, nil
	}
	if !ack {
		return nil, &errtypes.ConfigError{Msg: fmt.Sprintf(
			"an interrupted %s op on node %q is recorded (stopped before step %q); re-run with --acknowledge-interrupted-op to override it, or finish it first",
			marker.Op, marker.Target, marker.Step)}
	}
	r.Log.Warn("node: overwriting stranded op marker",
		"op", string(marker.Op), "node", marker.Target, "step", string(marker.Step))
	return nil, nil
}

// resizeScopeMatch builds the OpMatch for a resize resume. A single-node scope
// matches the marker naming that node; a role scope matches when the marker's
// recorded node resolves to the scoped role in the current node list, so an
// interrupted role roll resumes on the same role it started.
func resizeScopeMatch(scope ResizeScope, nodes []cluster.NodeDetail) OpMatch {
	return func(m *OpMarker) bool {
		if scope.Node != "" {
			return m.Target == scope.Node
		}
		for _, n := range nodes {
			if n.Name == m.Target {
				return n.Role == scope.Role
			}
		}
		return false
	}
}
