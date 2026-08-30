// Package node implements okdctl's node-lifecycle primitives — removal, resize,
// and compaction — atop terraform.Executor and cluster.Client.
// Every mutating step is guarded and recorded in an on-disk marker so an
// interrupted op is safe to re-run.
package node

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/marker"
)

// OpMarkerFileName is the per-op state marker filename under the work
// directory, mirroring deploy's StateFileName. It lets a crashed run resume
// with context and leaves a drained node cordoned.
const OpMarkerFileName = ".okdctl-node-op.json"

const opStateSchemaV1 = "v1"

const (
	tfVarWorkerCount    = "worker_count"
	defaultDrainTimeout = "10m"
	msgListNodes        = "list nodes"
	msgPersistTopology  = "persist topology"
)

// Op identifies which node-lifecycle verb is in flight.
type Op string

// Node-lifecycle op identifiers recorded in the marker; OpSnapshot is distinct
// from OpRemove so its bounded cordon/drain can't be mistaken for an in-flight
// remove.
const (
	OpRemove   Op = "remove"
	OpResize   Op = "resize"
	OpCompact  Op = "compact"
	OpStop     Op = "stop"
	OpStart    Op = "start"
	OpSnapshot Op = "snapshot"
	OpAdd      Op = "add"
)

// Step names the mutating step the marker was written before.
// Steps are idempotent, so resume safely re-runs from the recorded step; the
// value also drives operator diagnostics.
type Step string

// Mutating steps a marker can be written before; used for resume diagnostics.
const (
	StepCordon     Step = "cordon"
	StepDrain      Step = "drain"
	StepTFApply    Step = "tf-apply"
	StepPowerCycle Step = "power-cycle"
	StepDiskGrow   Step = "disk-grow"
	StepDeleteK8s  Step = "delete-node"
	StepUncordon   Step = "uncordon"
	StepShutdown   Step = "shutdown"
	StepPowerOn    Step = "power-on"

	// node add's own steps: StepTFApply is reused for the count apply; no
	// StepIgnitionDown because teardown is a defer that must run on every exit,
	// not just resumed ones.
	StepBuildISO   Step = "build-iso"
	StepUploadISO  Step = "upload-iso"
	StepIgnitionUp Step = "ignition-up"
	StepWaitJoin   Step = "wait-join"
)

// opState is the marker payload; Envelope.ClusterName guards against a marker
// left by a different cluster (mirrors deployState).
type opState struct {
	marker.Envelope

	Op     Op     `json:"op"`
	Target string `json:"target"`
	Step   Step   `json:"step"`
}

var opMarkerFile = marker.File{Label: "node op", Version: opStateSchemaV1}

// markStep writes the op marker before a mutating step; a write failure is
// fatal — losing it loses resume context and the leave-cordoned guarantee.
func markStep(path string, op Op, target string, step Step, runID, clusterName string) error {
	s := &opState{Op: op, Target: target, Step: step}
	if err := opMarkerFile.Write(path, s, runID, clusterName); err != nil {
		return fmt.Errorf("write op state marker: %w", err)
	}
	return nil
}

// readOpState returns nil when the marker is absent or fails the cluster-name
// guard (same reject posture as deploy's resume marker).
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
// non-mutating callers like `okdctl node list`. It mirrors opState's fields
// without exposing the marker's on-disk JSON shape.
type OpMarker struct {
	Op          Op
	Target      string
	Step        Step
	RunID       string
	ClusterName string
	Timestamp   time.Time
}

// CompletedAddResidue reports whether the marker is residue from a
// fully-completed add batch rather than an in-flight op. AddWorkers persists
// the widened worker count only after every node joins, so a target index below
// workerCount means the batch finished before the marker was cleared.
func (m *OpMarker) CompletedAddResidue(workerCount int) bool {
	if m.Op != OpAdd {
		return false
	}
	idx, ok := cluster.NodeIndex(m.Target)
	return ok && idx < workerCount
}

// ReadOpMarker returns nil when no op is in flight — no marker file, or one
// left by a different cluster. It's safe to call without the run lock: the
// marker is written via AtomicWrite, so a concurrent writer never leaves a
// partial read.
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
