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

func TestResolveResumePhase(t *testing.T) {
	tests := []struct {
		name       string
		seed       func(t *testing.T, path string)
		fresh      bool
		wantPhase  deployPhase
		wantMarker bool
	}{
		{
			name:      "no marker starts from prepare",
			seed:      func(*testing.T, string) {},
			wantPhase: phasePrepare,
		},
		{
			name: "prepare marker resumes through the wipe",
			seed: func(t *testing.T, path string) {
				if err := writeDeployState(path, phasePrepare, "run-1", "prod"); err != nil {
					t.Fatalf("writeDeployState: %v", err)
				}
			},
			wantPhase:  phasePrepare,
			wantMarker: true,
		},
		{
			name: "install marker routes past the wipe",
			seed: func(t *testing.T, path string) {
				if err := writeDeployState(path, phaseInstall, "run-2", "prod"); err != nil {
					t.Fatalf("writeDeployState: %v", err)
				}
			},
			wantPhase:  phaseInstall,
			wantMarker: true,
		},
		{
			name: "configure marker routes past wipe and install",
			seed: func(t *testing.T, path string) {
				if err := writeDeployState(path, phaseConfigure, "run-3", "prod"); err != nil {
					t.Fatalf("writeDeployState: %v", err)
				}
			},
			wantPhase:  phaseConfigure,
			wantMarker: true,
		},
		{
			name: "fresh overrides install marker and restarts from prepare",
			seed: func(t *testing.T, path string) {
				if err := writeDeployState(path, phaseInstall, "run-4", "prod"); err != nil {
					t.Fatalf("writeDeployState: %v", err)
				}
			},
			fresh:      true,
			wantPhase:  phasePrepare,
			wantMarker: true,
		},
		{
			name: "marker from different cluster is ignored",
			seed: func(t *testing.T, path string) {
				if err := writeDeployState(path, phaseInstall, "run-5", "other"); err != nil {
					t.Fatalf("writeDeployState: %v", err)
				}
			},
			wantPhase: phasePrepare,
		},
		{
			name: "corrupt marker treated as absent",
			seed: func(t *testing.T, path string) {
				if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
					t.Fatalf("seed file: %v", err)
				}
			},
			wantPhase: phasePrepare,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "deploy.state")
			tc.seed(t, path)
			phase, marker := resolveResumePhase(path, "prod", tc.fresh)
			if phase != tc.wantPhase {
				t.Errorf("phase = %q; want %q", phase, tc.wantPhase)
			}
			if (marker != nil) != tc.wantMarker {
				t.Errorf("marker = %+v; want present=%v", marker, tc.wantMarker)
			}
		})
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
