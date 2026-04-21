package terraform

import (
	"os"
	"path/filepath"
	"testing"
)

// TestExecutor_Cleanup_PreservesTFState locks that Executor.Cleanup removes
// plan files and the backup but NEVER touches terraform.tfstate. Mirrors
// cleanup.cleanupTerraformEnv's contract; the two must not drift.
func TestExecutor_Cleanup_PreservesTFState(t *testing.T) {
	workDir := t.TempDir()
	files := map[string]string{
		PlanFileName:               "plan",
		"destroy.tfplan":           "dplan",
		"terraform.tfstate.backup": "backup",
		"terraform.tfstate":        `{"version":4,"resources":[]}`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(workDir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	e := &Executor{WorkDir: workDir}
	if err := e.Cleanup(); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	mustBeGone := []string{PlanFileName, "destroy.tfplan", "terraform.tfstate.backup"}
	for _, f := range mustBeGone {
		if _, err := os.Stat(filepath.Join(workDir, f)); !os.IsNotExist(err) {
			t.Errorf("%s not removed: %v", f, err)
		}
	}

	// The invariant.
	body, err := os.ReadFile(filepath.Join(workDir, "terraform.tfstate"))
	if err != nil {
		t.Fatalf("terraform.tfstate removed (DATA LOSS): %v", err)
	}
	if string(body) != `{"version":4,"resources":[]}` {
		t.Errorf("terraform.tfstate mutated: %q", body)
	}
}

func TestExecutor_Cleanup_MissingFilesIgnored(t *testing.T) {
	workDir := t.TempDir()
	e := &Executor{WorkDir: workDir}
	if err := e.Cleanup(); err != nil {
		t.Errorf("expected nil for empty dir; got %v", err)
	}
}

func TestExecutor_HasState(t *testing.T) {
	dir := t.TempDir()
	e := &Executor{WorkDir: dir}

	if e.HasState() {
		t.Error("HasState() = true on empty dir")
	}

	if err := os.WriteFile(filepath.Join(dir, "terraform.tfstate"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !e.HasState() {
		t.Error("HasState() = false after tfstate written")
	}
}
