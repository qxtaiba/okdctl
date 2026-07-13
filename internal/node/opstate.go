// Package node implements okdctl's node-lifecycle primitives — worker removal,
// per-role resize, and the cluster-compaction orchestrator — on top of the
// terraform.Executor (VM mutations) and cluster.Client (Kubernetes lifecycle)
// layers. Every mutating step is fronted by a guard and recorded in an on-disk
// op marker so an interrupted op is safe to re-run.
package node

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/qxtaiba/okdctl/internal/system"
)

// OpMarkerFileName is the per-op state marker written under the work directory,
// mirroring deploy's StateFileName. It records the in-flight node op so a
// crashed run resumes with context and leaves a drained node cordoned.
const OpMarkerFileName = ".okdctl-node-op.json"

const opStateSchemaV1 = "v1"

// Op identifies which node-lifecycle verb is in flight.
type Op string

// Node-lifecycle op identifiers recorded in the marker.
const (
	OpRemove  Op = "remove"
	OpResize  Op = "resize"
	OpCompact Op = "compact"
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
	StepHAProxy    Step = "haproxy"
	StepUncordon   Step = "uncordon"
)

// opState is the marker payload. ClusterName guards against a marker left in a
// directory later reused for a different cluster (mirrors deployState).
type opState struct {
	SchemaVersion string    `json:"schema_version"`
	Op            Op        `json:"op"`
	Target        string    `json:"target"`
	Step          Step      `json:"step"`
	RunID         string    `json:"run_id"`
	ClusterName   string    `json:"cluster_name"`
	Timestamp     time.Time `json:"timestamp"`
}

// markStep writes the op marker before a mutating step runs. A write failure is
// fatal to the op: without the marker an interrupted run loses its resume
// context and the leave-cordoned guarantee.
func markStep(path string, op Op, target string, step Step, runID, clusterName string) error {
	data, err := json.Marshal(opState{
		SchemaVersion: opStateSchemaV1,
		Op:            op,
		Target:        target,
		Step:          step,
		RunID:         runID,
		ClusterName:   clusterName,
		Timestamp:     time.Now().UTC(),
	})
	if err != nil {
		return fmt.Errorf("marshal op state: %w", err)
	}
	if err := system.AtomicWrite(path, data, 0o600); err != nil {
		return fmt.Errorf("write op state marker: %w", err)
	}
	return nil
}

// readOpState reads the marker, returning nil when absent or when it names a
// different cluster (treated as absent, matching deploy's marker guard).
func readOpState(path, clusterName string) (*opState, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read op state: %w", err)
	}
	var s opState
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("parse op state: %w", err)
	}
	if s.ClusterName != "" && s.ClusterName != clusterName {
		return nil, nil
	}
	return &s, nil
}

// clearOpMarker removes the marker on clean completion; a missing file is fine.
func clearOpMarker(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove op state marker: %w", err)
	}
	return nil
}

func markerPath(workDir string) string {
	return filepath.Join(workDir, OpMarkerFileName)
}
