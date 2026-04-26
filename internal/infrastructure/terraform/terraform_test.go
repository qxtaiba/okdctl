package terraform

import (
	"os"
	"path/filepath"
	"testing"
)

// TestExecutor_CleanupPlans_PreservesBackupAndState locks that CleanupPlans
// removes only plan files and never touches terraform.tfstate or
// terraform.tfstate.backup — the backup is the operator's rollback artefact
// and must survive a successful run.
func TestExecutor_CleanupPlans_PreservesBackupAndState(t *testing.T) {
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
	if err := e.CleanupPlans(); err != nil {
		t.Fatalf("CleanupPlans: %v", err)
	}

	mustBeGone := []string{PlanFileName, "destroy.tfplan"}
	for _, f := range mustBeGone {
		if _, err := os.Stat(filepath.Join(workDir, f)); !os.IsNotExist(err) {
			t.Errorf("%s not removed: %v", f, err)
		}
	}

	mustSurvive := []string{"terraform.tfstate.backup", "terraform.tfstate"}
	for _, f := range mustSurvive {
		if _, err := os.Stat(filepath.Join(workDir, f)); err != nil {
			t.Fatalf("%s removed (DATA LOSS): %v", f, err)
		}
	}

	body, err := os.ReadFile(filepath.Join(workDir, "terraform.tfstate"))
	if err != nil {
		t.Fatalf("terraform.tfstate read after cleanup: %v", err)
	}
	if string(body) != `{"version":4,"resources":[]}` {
		t.Errorf("terraform.tfstate mutated: %q", body)
	}
}

func TestExecutor_CleanupPlans_MissingFilesIgnored(t *testing.T) {
	workDir := t.TempDir()
	e := &Executor{WorkDir: workDir}
	if err := e.CleanupPlans(); err != nil {
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
