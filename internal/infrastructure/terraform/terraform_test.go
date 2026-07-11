package terraform

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

type captureHandler struct {
	records []slog.Record
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error { //nolint:gocritic // hugeParam: slog.Handler interface requires value receiver
	h.records = append(h.records, r)
	return nil
}

func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *captureHandler) hasWarn() bool {
	for i := range h.records {
		if h.records[i].Level == slog.LevelWarn {
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

	e := &Executor{workDir: workDir}
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
	e := &Executor{workDir: workDir}
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

	e := &Executor{workDir: workDir, varFile: vf, logger: slog.New(&captureHandler{})}
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
		workDir: workDir,
		varFile: filepath.Join(workDir, "does-not-exist.tfvars"),
		logger:  slog.New(h),
	}

	got := e.buildVarArgs("", nil)

	for _, s := range got {
		if s == "-var-file="+e.varFile {
			t.Errorf("buildVarArgs included -var-file for missing file")
		}
	}
	if !h.hasWarn() {
		t.Error("expected a Warn log for missing var file; got none")
	}
}

func TestExecutor_HasState(t *testing.T) {
	cases := []struct {
		name     string
		content  string
		write    bool
		wantTrue bool
		wantWarn bool
	}{
		{name: "no file", wantTrue: false},
		{name: "empty JSON", content: `{}`, write: true, wantTrue: false},
		{name: "empty resources array", content: `{"version":4,"resources":[]}`, write: true, wantTrue: false},
		{name: "corrupt JSON", content: `{not valid json`, write: true, wantTrue: false, wantWarn: true},
		{name: "populated", content: `{"version":4,"resources":[{"type":"aws_instance"}]}`, write: true, wantTrue: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			h := &captureHandler{}
			e := &Executor{workDir: dir, logger: slog.New(h)}

			if tc.write {
				if err := os.WriteFile(filepath.Join(dir, "terraform.tfstate"), []byte(tc.content), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			if got := e.HasState(); got != tc.wantTrue {
				t.Errorf("HasState() = %v; want %v", got, tc.wantTrue)
			}
			if tc.wantWarn && !h.hasWarn() {
				t.Error("expected a Warn log; got none")
			}
			if !tc.wantWarn && h.hasWarn() {
				t.Errorf("unexpected Warn log emitted")
			}
		})
	}
}

func TestExecutor_StateStatus(t *testing.T) {
	cases := []struct {
		name    string
		content string
		write   bool
		want    StateStatusValue
	}{
		{name: "missing", want: StateStatusMissing},
		{name: "empty JSON", content: `{}`, write: true, want: StateStatusEmpty},
		{name: "empty resources", content: `{"version":4,"resources":[]}`, write: true, want: StateStatusEmpty},
		{name: "corrupt", content: `{not valid json`, write: true, want: StateStatusCorrupt},
		{name: "populated", content: `{"version":4,"resources":[{"type":"aws_instance"}]}`, write: true, want: StateStatusPopulated},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			e := &Executor{workDir: dir, logger: slog.New(&captureHandler{})}
			if tc.write {
				if err := os.WriteFile(filepath.Join(dir, "terraform.tfstate"), []byte(tc.content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if got := e.StateStatus(); got != tc.want {
				t.Errorf("StateStatus() = %q; want %q", got, tc.want)
			}
		})
	}
}

func installFakeTerraformOutput(t *testing.T, stdout string, exitCode int) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\ncat <<'EOF'\n" + stdout + "\nEOF\nexit " + strconv.Itoa(exitCode) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "terraform"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestExecutor_Output_ParsesJSON(t *testing.T) {
	installFakeTerraformOutput(t, `{"foo":{"value":"bar","type":"string"}}`, 0)
	e := New(t.TempDir())
	out, err := e.Output(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := out["foo"]; !ok {
		t.Errorf("output missing key %q; got %v", "foo", out)
	}
}

func TestExecutor_Output_NonZeroExit(t *testing.T) {
	installFakeTerraformOutput(t, "", 1)
	e := New(t.TempDir())
	if _, err := e.Output(context.Background()); err == nil {
		t.Fatal("expected error for non-zero exit")
	}
}

func TestExecutor_NewestBakSnapshot(t *testing.T) {
	t.Run("none present", func(t *testing.T) {
		e := &Executor{workDir: t.TempDir()}
		if got := e.NewestBakSnapshot(); got != "" {
			t.Errorf("NewestBakSnapshot() = %q; want empty", got)
		}
	})
	t.Run("returns newest by lexicographic name", func(t *testing.T) {
		dir := t.TempDir()
		names := []string{
			"terraform.tfstate.2024-01-01T00-00-00Z.bak",
			"terraform.tfstate.2024-01-03T00-00-00Z.bak",
			"terraform.tfstate.2024-01-02T00-00-00Z.bak",
		}
		for _, n := range names {
			if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		e := &Executor{workDir: dir}
		got := e.NewestBakSnapshot()
		want := filepath.Join(dir, "terraform.tfstate.2024-01-03T00-00-00Z.bak")
		if got != want {
			t.Errorf("NewestBakSnapshot() = %q; want %q", got, want)
		}
	})
}

func TestExecutor_PruneBakSnapshotsExceptNewest(t *testing.T) {
	t.Run("none present", func(t *testing.T) {
		e := &Executor{workDir: t.TempDir()}
		kept, err := e.PruneBakSnapshotsExceptNewest()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if kept != "" {
			t.Errorf("kept = %q; want empty", kept)
		}
	})

	t.Run("keeps newest, removes the rest", func(t *testing.T) {
		dir := t.TempDir()
		names := []string{
			"terraform.tfstate.2024-01-01T00-00-00Z.bak",
			"terraform.tfstate.2024-01-02T00-00-00Z.bak",
			"terraform.tfstate.2024-01-03T00-00-00Z.bak",
		}
		for _, n := range names {
			if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		e := &Executor{workDir: dir}
		kept, err := e.PruneBakSnapshotsExceptNewest()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(dir, "terraform.tfstate.2024-01-03T00-00-00Z.bak")
		if kept != want {
			t.Errorf("kept = %q; want %q", kept, want)
		}
		for _, n := range names[:2] {
			if _, statErr := os.Stat(filepath.Join(dir, n)); !os.IsNotExist(statErr) {
				t.Errorf("%s still present; want removed", n)
			}
		}
		if _, statErr := os.Stat(want); statErr != nil {
			t.Errorf("newest snapshot removed: %v", statErr)
		}
	})
}
