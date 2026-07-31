// Package node implements okdctl's node-lifecycle primitives — worker removal,
// per-role resize, and the cluster-compaction orchestrator — on top of the
// terraform.Executor (VM mutations) and cluster.Client (Kubernetes lifecycle)
// layers. Every mutating step is fronted by a guard and recorded in an on-disk
// op marker so an interrupted op is safe to re-run.
package node

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/marker"
)

// OpMarkerFileName is the per-op state marker written under the work directory,
// mirroring deploy's StateFileName. It records the in-flight node op so a
// crashed run resumes with context and leaves a drained node cordoned.
const OpMarkerFileName = ".okdctl-node-op.json"

const opStateSchemaV1 = "v1"

// Shared literals used across the node-lifecycle verbs: the terraform count
// variable, the default drain/operation timeout, and the ClusterError wrap
// labels reused by every verb's ListNodes and persist-topology steps.
const (
	tfVarWorkerCount    = "worker_count"
	defaultDrainTimeout = "10m"
	msgListNodes        = "list nodes"
	msgPersistTopology  = "persist topology"
)

// Op identifies which node-lifecycle verb is in flight.
type Op string

// Node-lifecycle op identifiers recorded in the marker. OpSnapshot is its own
// identifier, distinct from OpRemove, precisely so a snapshot's cordon/drain
// can never be mistaken for an in-flight remove by anything that later keys
// off Op equality — snapshots are bounded, non-resumable ops and must not
// borrow another op's resume identity.
const (
	OpRemove   Op = "remove"
	OpResize   Op = "resize"
	OpCompact  Op = "compact"
	OpStop     Op = "stop"
	OpStart    Op = "start"
	OpSnapshot Op = "snapshot"
	OpAdd      Op = "add"
)

// Step names the mutating step the marker was written before. Steps are
// idempotent, so resume re-runs from the recorded step without harm; the value
// exists for operator-facing diagnostics and to signal "left cordoned".
type Step string

// Mutating steps a marker can be written before; used for resume diagnostics.
const (
	StepCordon     Step = "cordon"
	StepDrain      Step = "drain"
	StepTFApply    Step = "tf-apply"
	StepPowerCycle Step = "power-cycle"
	StepDeleteK8s  Step = "delete-node"
	StepUncordon   Step = "uncordon"
	StepShutdown   Step = "shutdown"
	StepPowerOn    Step = "power-on"

	// node add's own step sequence; StepTFApply above is reused for the
	// worker_count apply. Ignition teardown is a plain defer that never calls
	// markStep (it must run on every exit path, not just a resumed one), so
	// there is no corresponding StepIgnitionDown.
	StepBuildISO   Step = "build-iso"
	StepUploadISO  Step = "upload-iso"
	StepIgnitionUp Step = "ignition-up"
	StepWaitJoin   Step = "wait-join"
)

// opState is the marker payload. The envelope's ClusterName guards against a
// marker left in a directory later reused for a different cluster (mirrors
// deployState).
type opState struct {
	marker.Envelope

	Op     Op     `json:"op"`
	Target string `json:"target"`
	Step   Step   `json:"step"`
}

var opMarkerFile = marker.File{Label: "node op", Version: opStateSchemaV1}

// markStep writes the op marker before a mutating step runs. A write failure is
// fatal to the op: without the marker an interrupted run loses its resume
// context and the leave-cordoned guarantee.
func markStep(path string, op Op, target string, step Step, runID, clusterName string) error {
	s := &opState{Op: op, Target: target, Step: step}
	if err := opMarkerFile.Write(path, s, runID, clusterName); err != nil {
		return fmt.Errorf("write op state marker: %w", err)
	}
	return nil
}

// readOpState reads the marker, returning nil when absent or when it fails
// the cluster-name guard (empty or mismatching names are treated as absent —
// the same reject posture as deploy's resume marker).
func readOpState(path, clusterName string) (*opState, error) {
	var s opState
	found, err := opMarkerFile.Read(path, &s)
	if err != nil || !found {
		return nil, err
	}
	if !opMarkerFile.Trusted(&s, clusterName) {
		return nil, nil
	}
	if s.Stale() {
		logutil.Warn("node op marker is likely stale",
			logutil.LF("op", string(s.Op)),
			logutil.LF("marker_age", s.Age().Round(time.Hour).String()))
	}
	return &s, nil
}

// clearOpMarker removes the marker on clean completion; a missing file is fine.
func clearOpMarker(path string) error {
	return opMarkerFile.Clear(path)
}

func markerPath(workDir string) string {
	return filepath.Join(workDir, OpMarkerFileName)
}

// OpMarker is the read-only view of an in-flight node op exposed to
// non-mutating callers (`okdctl node list`). It mirrors opState's fields
// without exposing the marker's on-disk JSON shape as public API.
type OpMarker struct {
	Op          Op
	Target      string
	Step        Step
	RunID       string
	ClusterName string
	Timestamp   time.Time
}

// ReadOpMarker reads the op marker under workDir, returning nil when no op is
// in flight (no marker file, or a marker left by a different cluster). Safe to
// call without holding the run lock: the marker is written via AtomicWrite, so
// a concurrent writer never leaves a partial read.
func ReadOpMarker(workDir, clusterName string) (*OpMarker, error) {
	s, err := readOpState(markerPath(workDir), clusterName)
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, nil
	}
	return &OpMarker{
		Op:          s.Op,
		Target:      s.Target,
		Step:        s.Step,
		RunID:       s.RunID,
		ClusterName: s.ClusterName,
		Timestamp:   s.Timestamp,
	}, nil
}
