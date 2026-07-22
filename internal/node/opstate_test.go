package node

import (
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
