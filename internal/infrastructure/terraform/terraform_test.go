package terraform

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

type captureHandler struct {
	records []slog.Record
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r)
	return nil
}

func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *captureHandler) hasWarn() bool {
	for _, r := range h.records {
		if r.Level == slog.LevelWarn {
			return true
		}
	}
	return false
}

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

func TestExecutor_BuildVarArgs_DeterministicOrder(t *testing.T) {
	workDir := t.TempDir()
	vf := filepath.Join(workDir, "terraform.tfvars")
	if err := os.WriteFile(vf, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}

	e := &Executor{WorkDir: workDir, VarFile: vf, logger: slog.New(&captureHandler{})}
	got := e.buildVarArgs("", map[string]string{"z": "3", "a": "1", "m": "2"})

	want := []string{"-var-file=" + vf, "-var", "a=1", "-var", "m=2", "-var", "z=3"}
	if len(got) != len(want) {
		t.Fatalf("buildVarArgs len = %d; want %d\ngot:  %v\nwant: %v", len(got), len(want), got, want)
	}
	for i, v := range want {
		if got[i] != v {
			t.Errorf("buildVarArgs[%d] = %q; want %q", i, got[i], v)
		}
	}
}

func TestExecutor_BuildVarArgs_VarFileMissing(t *testing.T) {
	workDir := t.TempDir()
	h := &captureHandler{}
	e := &Executor{
		WorkDir: workDir,
		VarFile: filepath.Join(workDir, "does-not-exist.tfvars"),
		logger:  slog.New(h),
	}

	got := e.buildVarArgs("", nil)

	for _, s := range got {
		if s == "-var-file="+e.VarFile {
			t.Errorf("buildVarArgs included -var-file for missing file")
		}
	}
	if !h.hasWarn() {
		t.Error("expected a Warn log for missing var file; got none")
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
