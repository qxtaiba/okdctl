package node

import (
	"fmt"
	"slices"

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
//
// StepIgnitionUp sorts BEFORE StepBuildISO: the ignition revive is a
// batch-scoped step recorded against the batch's first node, so a resume that
// reattaches to it (crash right after revive, before any per-node work) must
// still re-run that node's build/upload/apply, not skip them.
var stepOrder = map[Step]int{
	StepCordon:     0,
	StepDrain:      1,
	StepTFApply:    2,
	StepPowerCycle: 3,
	StepDeleteK8s:  4,
	StepUncordon:   5,

	StepIgnitionUp: -4,
	StepBuildISO:   -3,
	StepUploadISO:  -2,
	StepWaitJoin:   6,
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
// and the confirm gate. Every caller gates it on !DryRun, so its only
// mutation — sweeping a completed add batch's leftover marker — never runs
// under the dry-run zero-mutation contract. It classifies the on-disk marker
// into one of four outcomes:
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

// sweepCompletedAddMarker clears (and reports true for) a marker that is the
// residue of a fully-completed add batch: AddWorkers persists the widened
// worker count only after every node in the batch has joined, so a marker
// whose worker index precedes the persisted count can only mean the batch
// finished and the process died between persistTopology and clearOpMarker.
// Without this sweep every subsequent op refuses on a batch that actually
// completed, with a misleading "stopped before step wait-join" message. A
// genuinely in-flight add always carries an index at or above the persisted
// count (its batch starts there), so it is never swept.
func (r *Runner) sweepCompletedAddMarker(m *OpMarker) bool {
	if m.Op != OpAdd {
		return false
	}
	idx, ok := cluster.NodeIndex(m.Target)
	if !ok || idx >= r.Cfg.Topology.Workers.Count {
		return false
	}
	r.Log.Info("node: clearing marker left by a completed add batch",
		"node", m.Target, "step", string(m.Step))
	if err := clearOpMarker(r.marker()); err != nil {
		r.Log.Warn("node: op marker cleanup failed", "err", err)
	}
	return true
}

// strandedMarkerMsg renders the refusal for a stranded op marker. An add's
// marker gets an extra warning: the interrupted add may have left the ignition
// server (which serves the cluster pull secret) running, and nothing else
// surfaces that exposure to the operator.
func strandedMarkerMsg(m *OpMarker) string {
	scope := fmt.Sprintf("an interrupted %s op on node %q", m.Op, m.Target)
	if m.Op == OpStart {
		// start's marker is cluster-scoped: its Target records the cluster
		// name, not a node.
		scope = fmt.Sprintf("an interrupted cluster start op for cluster %q", m.Target)
	}
	msg := fmt.Sprintf("%s is recorded (stopped before step %q); re-run with --acknowledge-interrupted-op to override it, or finish it first",
		scope, m.Step)
	if m.Op == OpAdd {
		msg += "; the interrupted add may have left the ignition server running — check 'systemctl status httpd' and stop it if no add is in progress"
	}
	return msg
}

// refuseForeignMarker refuses (unless ack) when an on-disk marker records an
// op the caller does not compose. It is the shared guard behind every
// non-resumable op (snapshot create/rollback, cluster stop/start) and behind
// compact's own pre-mutation check: called with no allowResumable, ANY
// existing marker is foreign and refused; compact calls it with
// allowResumable=OpRemove,OpResize since a marker for either of its own inner
// ops is indistinguishable from compact's own in-flight call (one cluster per
// workdir) and refusing it would break compact's resume — see
// refuseForeignMarkerBeforeCompact. Unlike beginOp this never returns a
// marker to resume from: every caller of this guard is non-resumable itself.
// With ack, a foreign marker is warned about and deleted here rather than
// left for a later markStep overwrite — callers may finish without writing
// any marker of their own. Callers gate on !DryRun, keeping that delete out
// of the dry-run zero-mutation contract.
func (r *Runner) refuseForeignMarker(ack bool, allowResumable ...Op) error {
	marker, err := ReadOpMarker(r.WorkDir, r.Cfg.Cluster.Name)
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
	// Consume the acknowledged marker instead of leaving it for the op's own
	// markStep to overwrite: stop/start/snapshot can finish without ever
	// writing a marker of their own (--skip-drain, a NotReady target), and a
	// marker the operator already acknowledged must not resurface on the
	// next op.
	r.Log.Warn("node: discarding stranded op marker",
		"op", string(marker.Op), "node", marker.Target, "step", string(marker.Step))
	if cerr := clearOpMarker(r.marker()); cerr != nil {
		r.Log.Warn("node: op marker cleanup failed", "err", cerr)
	}
	return nil
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
