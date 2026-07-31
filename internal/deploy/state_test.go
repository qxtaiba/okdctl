package deploy

import (
	"fmt"
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

// seedRawState writes the literal JSON older binaries produced, so the
// migration tests exercise real on-disk bytes rather than the current
// struct's marshal output.
func seedRawState(t *testing.T, path, schema string, phase deployPhase) {
	t.Helper()
	raw := fmt.Sprintf(
		`{"schema_version":%q,"phase":%q,"run_id":"run-xyz","timestamp":"2026-05-01T10:00:00Z","cluster_name":"prod"}`,
		schema, phase)
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
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
		{
			name: "completed marker treated as absent, never grants resume",
			seed: func(t *testing.T, path string) {
				if err := writeDeployState(path, phaseCompleted, "run-8", "prod"); err != nil {
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

func TestInstallInProgress(t *testing.T) {
	seed := func(t *testing.T, phase deployPhase, cluster string) string {
		t.Helper()
		dir := t.TempDir()
		if err := writeDeployState(filepath.Join(dir, StateFileName), phase, "run-1", cluster); err != nil {
			t.Fatalf("writeDeployState: %v", err)
		}
		return dir
	}

	if InstallInProgress(t.TempDir(), "prod") {
		t.Error("no marker: want false")
	}
	if !InstallInProgress(seed(t, phaseInstall, "prod"), "prod") {
		t.Error("install marker: want true")
	}
	if !InstallInProgress(seed(t, phaseSetup, "prod"), "prod") {
		t.Error("setup marker: want true")
	}
	if InstallInProgress(seed(t, phaseCompleted, "prod"), "prod") {
		t.Error("completed marker: want false")
	}
	if InstallInProgress(seed(t, phaseInstall, "other"), "prod") {
		t.Error("foreign-cluster marker: want false")
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

func TestClearDeployMarker_RemovesMarker(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deploy.state")
	if err := writeDeployState(path, phasePostInstall, "run-x", "prod"); err != nil {
		t.Fatalf("writeDeployState: %v", err)
	}
	clearDeployMarker(path, "run-x", "prod")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("marker still present after clear: %v", err)
	}
	// Absent marker: must be a silent no-op.
	clearDeployMarker(path, "run-x", "prod")
}

// TestCompletedMarker_IsInertEverywhere locks the terminal marker contract:
// a completed-phase marker written by clearDeployMarker's remove-failure
// fallback must behave exactly like no marker for both resume routing and
// destroy diagnostics.
func TestCompletedMarker_IsInertEverywhere(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deploy.state")
	if err := writeDeployState(path, phaseCompleted, "run-done", "prod"); err != nil {
		t.Fatalf("writeDeployState: %v", err)
	}

	phase, marker := resolveResumePhase(path, "prod", false)
	if phase != phaseSetup || marker != nil {
		t.Errorf("resolveResumePhase = (%q, %+v); want (setup, nil)", phase, marker)
	}
	// must not print the partial-deploy advisory or panic
	AnnounceState(path, "prod")
}
