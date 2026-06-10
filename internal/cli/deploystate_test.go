package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReadDeployState_MissingFile(t *testing.T) {
	dir := t.TempDir()
	ds, err := readDeployState(filepath.Join(dir, "deploy.state"))
	if err != nil {
		t.Fatalf("missing file: want nil error, got %v", err)
	}
	if ds != nil {
		t.Fatalf("missing file: want nil state, got %+v", ds)
	}
}

func TestReadDeployState_CorruptJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deploy.state")
	if err := os.WriteFile(path, []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	_, err := readDeployState(path)
	if err == nil {
		t.Fatal("corrupt JSON: want error, got nil")
	}
}

func TestReadDeployState_V1RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deploy.state")
	if err := writeDeployState(path, phaseInstall, "run-abc", "prod"); err != nil {
		t.Fatalf("writeDeployState: %v", err)
	}
	ds, err := readDeployState(path)
	if err != nil {
		t.Fatalf("readDeployState: %v", err)
	}
	if ds == nil {
		t.Fatal("want non-nil deployState, got nil")
	}
	if ds.SchemaVersion != deployStateSchemaV1 {
		t.Errorf("SchemaVersion = %q; want %q", ds.SchemaVersion, deployStateSchemaV1)
	}
	if ds.Phase != phaseInstall {
		t.Errorf("Phase = %q; want %q", ds.Phase, phaseInstall)
	}
	if ds.RunID != "run-abc" {
		t.Errorf("RunID = %q; want run-abc", ds.RunID)
	}
	if ds.ClusterName != "prod" {
		t.Errorf("ClusterName = %q; want prod", ds.ClusterName)
	}
	if ds.Timestamp.IsZero() {
		t.Error("Timestamp must not be zero")
	}
}

func TestReadDeployState_UnknownSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deploy.state")
	raw := deployState{
		SchemaVersion: "v99",
		Phase:         phasePrepare,
		RunID:         "run-xyz",
		ClusterName:   "prod",
	}
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	ds, err := readDeployState(path)
	if err != nil {
		t.Fatalf("unknown schema: want nil error, got %v", err)
	}
	if ds != nil {
		t.Fatalf("unknown schema: want nil state, got %+v", ds)
	}
}

func TestAnnounceDeployState_ClusterMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deploy.state")
	if err := writeDeployState(path, phaseInstall, "run-abc", "cluster-a"); err != nil {
		t.Fatalf("writeDeployState: %v", err)
	}
	// must not panic and must return without printing the install advisory
	announceDeployState(path, "cluster-b")
}

func TestAnnounceDeployState_NoMarker(t *testing.T) {
	dir := t.TempDir()
	announceDeployState(filepath.Join(dir, "deploy.state"), "any-cluster")
}

func TestAnnounceDeployState_PreparePhase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deploy.state")
	if err := writeDeployState(path, phasePrepare, "run-prep", "prod"); err != nil {
		t.Fatalf("writeDeployState: %v", err)
	}
	announceDeployState(path, "prod")
}

func TestAnnounceDeployState_InstallPhase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deploy.state")
	if err := writeDeployState(path, phaseInstall, "run-inst", "prod"); err != nil {
		t.Fatalf("writeDeployState: %v", err)
	}
	announceDeployState(path, "prod")
}
