package node

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, OpMarkerFileName)

	if err := markStep(path, OpRemove, testWorkerNode, StepDrain, "run-1", "grappleberry"); err != nil {
		t.Fatalf("markStep: %v", err)
	}
	got, err := readOpState(path, "grappleberry")
	if err != nil {
		t.Fatalf("readOpState: %v", err)
	}
	if got == nil || got.Op != OpRemove || got.Target != testWorkerNode || got.Step != StepDrain {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestReadOpStateWrongClusterTreatedAbsent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, OpMarkerFileName)
	if err := markStep(path, OpResize, "master0", StepTFApply, "run-1", "clusterA"); err != nil {
		t.Fatalf("markStep: %v", err)
	}
	got, err := readOpState(path, "clusterB")
	if err != nil {
		t.Fatalf("readOpState: %v", err)
	}
	if got != nil {
		t.Fatalf("marker from another cluster must read as absent, got %+v", got)
	}
}

func TestReadOpStateMissing(t *testing.T) {
	got, err := readOpState(filepath.Join(t.TempDir(), "nope.json"), "x")
	if err != nil || got != nil {
		t.Fatalf("missing marker: want (nil,nil), got (%v,%v)", got, err)
	}
}

// TestReadOpState_LegacyV1RawMarker locks migration compatibility: the
// literal JSON bytes pre-unification binaries wrote must keep loading.
func TestReadOpState_LegacyV1RawMarker(t *testing.T) {
	path := filepath.Join(t.TempDir(), OpMarkerFileName)
	raw := `{"schema_version":"v1","op":"resize","target":"master0","step":"tf-apply","run_id":"run-1","cluster_name":"clusterA","timestamp":"2026-05-01T10:00:00Z"}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	got, err := readOpState(path, "clusterA")
	if err != nil {
		t.Fatalf("readOpState: %v", err)
	}
	if got == nil || got.Op != OpResize || got.Target != "master0" || got.Step != StepTFApply {
		t.Fatalf("legacy marker mismatch: %+v", got)
	}
	if got.RunID != "run-1" || got.ClusterName != "clusterA" || got.Timestamp.IsZero() {
		t.Fatalf("legacy envelope mismatch: %+v", got)
	}
}

func TestReadOpStateEmptyClusterNameTreatedAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), OpMarkerFileName)
	raw := `{"schema_version":"v1","op":"remove","target":"worker0","step":"drain","run_id":"run-1","cluster_name":"","timestamp":"2026-05-01T10:00:00Z"}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	got, err := readOpState(path, "clusterA")
	if err != nil {
		t.Fatalf("readOpState: %v", err)
	}
	if got != nil {
		t.Fatalf("marker without cluster name must read as absent, got %+v", got)
	}
}

func TestReadOpStateUnknownSchemaTreatedAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), OpMarkerFileName)
	raw := `{"schema_version":"v99","op":"remove","target":"worker0","step":"drain","run_id":"run-1","cluster_name":"clusterA","timestamp":"2026-05-01T10:00:00Z"}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	got, err := readOpState(path, "clusterA")
	if err != nil {
		t.Fatalf("readOpState: %v", err)
	}
	if got != nil {
		t.Fatalf("unknown-schema marker must read as absent, got %+v", got)
	}
}

func TestClearOpMarker(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, OpMarkerFileName)
	if err := markStep(path, OpRemove, "worker0", StepCordon, "r", "c"); err != nil {
		t.Fatalf("markStep: %v", err)
	}
	if err := clearOpMarker(path); err != nil {
		t.Fatalf("clearOpMarker: %v", err)
	}
	// second clear on absent file is a no-op
	if err := clearOpMarker(path); err != nil {
		t.Fatalf("clearOpMarker idempotent: %v", err)
	}
}
