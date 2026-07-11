package deploy

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

func TestReadDeployState_V2RoundTrip(t *testing.T) {
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
	if ds.SchemaVersion != deployStateSchemaV2 {
		t.Errorf("SchemaVersion = %q; want %q", ds.SchemaVersion, deployStateSchemaV2)
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

func seedRawState(t *testing.T, path, schema string, phase deployPhase) {
	t.Helper()
	data, err := json.Marshal(deployState{
		SchemaVersion: schema,
		Phase:         phase,
		RunID:         "run-xyz",
		ClusterName:   "prod",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}
}

func TestReadDeployState_V1PhaseMigration(t *testing.T) {
	tests := []struct {
		name      string
		v1Phase   deployPhase
		wantPhase deployPhase
	}{
		{name: "v1 prepare maps to setup", v1Phase: "prepare", wantPhase: phaseSetup},
		{name: "v1 configure maps to postinstall", v1Phase: "configure", wantPhase: phasePostInstall},
		{name: "v1 install unchanged", v1Phase: phaseInstall, wantPhase: phaseInstall},
		{name: "v1 unknown phase passes through", v1Phase: "someday", wantPhase: "someday"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "deploy.state")
			seedRawState(t, path, deployStateSchemaV1, tc.v1Phase)
			ds, err := readDeployState(path)
			if err != nil {
				t.Fatalf("readDeployState: %v", err)
			}
			if ds == nil {
				t.Fatal("want non-nil deployState, got nil")
			}
			if ds.Phase != tc.wantPhase {
				t.Errorf("Phase = %q; want %q", ds.Phase, tc.wantPhase)
			}
		})
	}
}

func TestResolveResumePhase_V1ConfigureMarkerResumesPostInstall(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deploy.state")
	seedRawState(t, path, deployStateSchemaV1, "configure")
	phase, marker := resolveResumePhase(path, "prod", false)
	if phase != phasePostInstall {
		t.Errorf("phase = %q; want %q", phase, phasePostInstall)
	}
	if marker == nil {
		t.Error("want non-nil marker")
	}
}

func TestReadDeployState_UnknownSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deploy.state")
	seedRawState(t, path, "v99", phaseSetup)
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
			name:      "no marker starts from setup",
			seed:      func(*testing.T, string) {},
			wantPhase: phaseSetup,
		},
		{
			name: "setup marker resumes through the wipe",
			seed: func(t *testing.T, path string) {
				if err := writeDeployState(path, phaseSetup, "run-1", "prod"); err != nil {
					t.Fatalf("writeDeployState: %v", err)
				}
			},
			wantPhase:  phaseSetup,
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
			name: "postinstall marker routes past wipe and install",
			seed: func(t *testing.T, path string) {
				if err := writeDeployState(path, phasePostInstall, "run-3", "prod"); err != nil {
					t.Fatalf("writeDeployState: %v", err)
				}
			},
			wantPhase:  phasePostInstall,
			wantMarker: true,
		},
		{
			name: "fresh overrides install marker and restarts from setup",
			seed: func(t *testing.T, path string) {
				if err := writeDeployState(path, phaseInstall, "run-4", "prod"); err != nil {
					t.Fatalf("writeDeployState: %v", err)
				}
			},
			fresh:      true,
			wantPhase:  phaseSetup,
			wantMarker: true,
		},
		{
			name: "marker from different cluster is ignored",
			seed: func(t *testing.T, path string) {
				if err := writeDeployState(path, phaseInstall, "run-5", "other"); err != nil {
					t.Fatalf("writeDeployState: %v", err)
				}
			},
			wantPhase: phaseSetup,
		},
		{
			name: "corrupt marker treated as absent",
			seed: func(t *testing.T, path string) {
				if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
					t.Fatalf("seed file: %v", err)
				}
			},
			wantPhase: phaseSetup,
		},
		{
			name: "marker without cluster name treated as absent",
			seed: func(t *testing.T, path string) {
				if err := writeDeployState(path, phaseInstall, "run-6", ""); err != nil {
					t.Fatalf("writeDeployState: %v", err)
				}
			},
			wantPhase: phaseSetup,
		},
		{
			name: "unknown marker phase treated as absent, no guard bypass",
			seed: func(t *testing.T, path string) {
				if err := writeDeployState(path, deployPhase("someday"), "run-7", "prod"); err != nil {
					t.Fatalf("writeDeployState: %v", err)
				}
			},
			wantPhase: phaseSetup,
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
	AnnounceState(path, "cluster-b")
}

func TestAnnounceDeployState_NoMarker(t *testing.T) {
	dir := t.TempDir()
	AnnounceState(filepath.Join(dir, "deploy.state"), "any-cluster")
}

func TestAnnounceDeployState_SetupPhase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deploy.state")
	if err := writeDeployState(path, phaseSetup, "run-prep", "prod"); err != nil {
		t.Fatalf("writeDeployState: %v", err)
	}
	AnnounceState(path, "prod")
}

func TestAnnounceDeployState_InstallPhase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deploy.state")
	if err := writeDeployState(path, phaseInstall, "run-inst", "prod"); err != nil {
		t.Fatalf("writeDeployState: %v", err)
	}
	AnnounceState(path, "prod")
}
