package terraform

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/qxtaiba/okdctl/internal/testutil"
)

func mustWriteFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func bakName(day int) string {
	return fmt.Sprintf("terraform.tfstate.2024-01-%02dT00-00-00Z.bak", day)
}

func writeBaks(t *testing.T, dir string, days ...int) []string {
	t.Helper()
	names := make([]string, len(days))
	for i, d := range days {
		names[i] = bakName(d)
		mustWriteFile(t, dir, names[i], "x")
	}
	return names
}

func TestExecutor_CleanupPlans_PreservesBackupAndState(t *testing.T) {
	workDir := t.TempDir()
	files := map[string]string{
		PlanFileName:               "plan",
		"destroy.tfplan":           "dplan",
		"terraform.tfstate.backup": "backup",
		"terraform.tfstate":        `{"version":4,"resources":[]}`,
	}
	for name, body := range files {
		mustWriteFile(t, workDir, name, body)
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
	vf := mustWriteFile(t, workDir, "terraform.tfvars", "")

	e := &Executor{workDir: workDir, varFile: vf, logger: slog.New(&testutil.CaptureHandler{})}
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
	h := &testutil.CaptureHandler{}
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
	if !h.HasLevel(slog.LevelWarn) {
		t.Error("expected a Warn log for missing var file; got none")
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
			e := &Executor{workDir: dir, logger: slog.New(&testutil.CaptureHandler{})}
			if tc.write {
				mustWriteFile(t, dir, "terraform.tfstate", tc.content)
			}
			if got := e.StateStatus(); got != tc.want {
				t.Errorf("StateStatus() = %q; want %q", got, tc.want)
			}
		})
	}
}

func installFakeTerraformOutput(t *testing.T, stdout string, exitCode int) {
	t.Helper()
	testutil.InstallFakeBin(t, "terraform", `#!/bin/sh
printf '%s\n' "${TF_FAKE_STDOUT:-}"
exit "${TF_FAKE_EXIT:-0}"
`)
	t.Setenv("TF_FAKE_STDOUT", stdout)
	t.Setenv("TF_FAKE_EXIT", strconv.Itoa(exitCode))
}

func TestExecutor_Output(t *testing.T) {
	t.Run("parses JSON", func(t *testing.T) {
		installFakeTerraformOutput(t, `{"foo":{"value":"bar","type":"string"}}`, 0)
		out, err := New(t.TempDir()).Output(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := out["foo"]; !ok {
			t.Errorf("output missing key %q; got %v", "foo", out)
		}
	})
	t.Run("non-zero exit", func(t *testing.T) {
		installFakeTerraformOutput(t, "", 1)
		if _, err := New(t.TempDir()).Output(context.Background()); err == nil {
			t.Fatal("expected error for non-zero exit")
		}
	})
}

func TestExecutor_PlanDetailed(t *testing.T) {
	cases := []struct {
		name        string
		exitCode    int
		wantChanges bool
		wantErr     bool
	}{
		{name: "exit 0 no changes", exitCode: 0, wantChanges: false, wantErr: false},
		{name: "exit 2 changes present", exitCode: 2, wantChanges: true, wantErr: false},
		{name: "exit 1 real failure", exitCode: 1, wantChanges: false, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			installFakeTerraformOutput(t, "", tc.exitCode)
			e := New(t.TempDir())
			gotChanges, err := e.PlanDetailed(context.Background(), PlanOptions{})
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotChanges != tc.wantChanges {
				t.Errorf("PlanDetailed() changes = %v; want %v", gotChanges, tc.wantChanges)
			}
		})
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
		writeBaks(t, dir, 1, 3, 2) // deliberately written out of order
		e := &Executor{workDir: dir}
		got := e.NewestBakSnapshot()
		want := filepath.Join(dir, bakName(3))
		if got != want {
			t.Errorf("NewestBakSnapshot() = %q; want %q", got, want)
		}
	})
}

func TestExecutor_PruneBakSnapshotsExceptNewest(t *testing.T) {
	dir := t.TempDir()
	names := writeBaks(t, dir, 1, 2, 3)
	e := &Executor{workDir: dir}
	kept, err := e.PruneBakSnapshotsExceptNewest()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(dir, bakName(3))
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
}
