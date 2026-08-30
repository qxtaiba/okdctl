package node

import (
	"fmt"
	"slices"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/errtypes"
)

// opMatch reports whether an on-disk marker belongs to the caller's op; beginOp
// resumes only when both Op and this match.
type opMatch func(m *OpMarker) bool

// stepOrder totally orders the mutating-step vocabulary for shouldRunStep: a
// resumed op re-runs its recorded step and everything after. Compared only
// within one op's own resume, never as a shared timeline — add reuses
// StepTFApply's slot and resize's StepDiskGrow shares remove's slot 4, since
// those ops never compare each other's steps. The negative ignition/ISO slots
// sort StepIgnitionUp before StepBuildISO so a crash right after the
// batch-scoped revive still replays that node's build/upload/apply.
var stepOrder = map[Step]int{
	StepCordon:     0,
	StepDrain:      1,
	StepTFApply:    2,
	StepPowerCycle: 3,
	StepDiskGrow:   4,
	StepDeleteK8s:  4,
	StepUncordon:   5,

	StepIgnitionUp: -4,
	StepBuildISO:   -3,
	StepUploadISO:  -2,
	StepWaitJoin:   6,
}

// shouldRunStep reports whether step runs given resume point from; "" means fresh (run everything).
func shouldRunStep(step, from Step) bool {
	if from == "" {
		return true
	}
	return stepOrder[step] >= stepOrder[from]
}

// runStep gates step on resumeStep, marks it BEFORE running fn (resume
// semantics depend on that order).
func (r *Runner) runStep(op Op, target string, step, resumeStep Step, fn func() error) error {
	if !shouldRunStep(step, resumeStep) {
		return nil
	}
	if err := r.mark(op, target, step); err != nil {
		return err
	}
	return fn()
}

// beginOp is the first call in every mutating op (before guards/confirm), gated
// by callers on !DryRun:
//   - none: (nil, nil), run normally.
//   - matching op+target: (marker, nil), resume — caller skips guards/confirm.
//   - foreign, ack=false: ConfigError naming the stranded op.
//   - foreign, ack=true: warn, (nil, nil) — proceeds fresh.
func (r *Runner) beginOp(op Op, match opMatch, ack bool) (*OpMarker, error) {
	marker, err := ReadOpMarker(r.workDir, r.Cfg.Cluster.Name)
	if err != nil {
		return nil, err
	}
	if marker == nil {
		return nil, nil
	}
	if r.sweepCompletedAddMarker(marker) {
		return nil, nil
	}
	if marker.Op == op && match(marker) {
		r.Log.Info("node: resuming interrupted op",
			"op", string(marker.Op), "node", marker.Target, "step", string(marker.Step))
		return marker, nil
	}
	if !ack {
		return nil, &errtypes.ConfigError{Msg: strandedMarkerMsg(marker)}
	}
	r.Log.Warn("node: overwriting stranded op marker",
		"op", string(marker.Op), "node", marker.Target, "step", string(marker.Step))
	return nil, nil
}

// sweepCompletedAddMarker clears a marker left by a fully-completed add batch:
// AddWorkers persists the widened count only after every node joins, so an
// index preceding the persisted count means the batch already finished.
func (r *Runner) sweepCompletedAddMarker(m *OpMarker) bool {
	if !m.CompletedAddResidue(r.Cfg.Topology.Workers.Count) {
		return false
	}
	r.Log.Info("node: clearing marker left by a completed add batch",
		"node", m.Target, "step", string(m.Step))
	if err := clearOpMarker(r.marker()); err != nil {
		r.Log.Warn("node: op marker cleanup failed", "err", err)
	}
	return true
}

// strandedMarkerMsg renders the stranded-marker refusal; an add marker gets an
// extra pull-secret-server warning.
func strandedMarkerMsg(m *OpMarker) string {
	scope := fmt.Sprintf("an interrupted %s op on node %q", m.Op, m.Target)
	if m.Op == OpStart {
		// start's marker is cluster-scoped: Target records the cluster name, not a node.
		scope = fmt.Sprintf("an interrupted cluster start op for cluster %q", m.Target)
	}
	msg := fmt.Sprintf("%s is recorded (stopped before step %q); re-run with --acknowledge-interrupted-op to override it, or finish it first",
		scope, m.Step)
	if m.Op == OpAdd {
		msg += "; the interrupted add may have left the ignition server running — check 'systemctl status httpd' and stop it if no add is in progress"
	}
	return msg
}

// refuseForeignMarker refuses (unless ack) any marker for an op the caller
// doesn't compose. compact passes allowResumable=OpRemove,OpResize since its
// own inner marker is indistinguishable from its in-flight call (see Compact).
func (r *Runner) refuseForeignMarker(ack bool, allowResumable ...Op) error {
	marker, err := ReadOpMarker(r.workDir, r.Cfg.Cluster.Name)
	if err != nil {
		if !ack {
			return err
		}
		r.Log.Warn("node: discarding unreadable op marker", "err", err)
		if cerr := clearOpMarker(r.marker()); cerr != nil {
			r.Log.Warn("node: op marker cleanup failed", "err", cerr)
		}
		return nil
	}
	if marker == nil || slices.Contains(allowResumable, marker.Op) {
		return nil
	}
	if r.sweepCompletedAddMarker(marker) {
		return nil
	}
	if !ack {
		return &errtypes.ConfigError{Msg: strandedMarkerMsg(marker)}
	}
	// Consumed here (not left for markStep): stop/start/snapshot can finish without writing their own.
	r.Log.Warn("node: discarding stranded op marker",
		"op", string(marker.Op), "node", marker.Target, "step", string(marker.Step))
	if cerr := clearOpMarker(r.marker()); cerr != nil {
		r.Log.Warn("node: op marker cleanup failed", "err", cerr)
	}
	return nil
}

// resizeScopeMatch builds the opMatch for a resize resume: node scope matches
// that node, role scope matches by role.
func resizeScopeMatch(scope ResizeScope, nodes []cluster.NodeDetail) opMatch {
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
