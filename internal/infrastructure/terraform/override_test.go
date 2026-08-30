package terraform

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
)

// seedWorkspaceLayout builds the pinned environments/modules workspace shape and returns both dirs.
func seedWorkspaceLayout(t *testing.T) (envDir, moduleDir string) {
	t.Helper()
	root := t.TempDir()
	envDir = filepath.Join(root, "infrastructure", "terraform", "environments", "production")
	moduleDir = filepath.Join(root, "infrastructure", "terraform", "modules", "proxmox-okd")
	for _, d := range []string{envDir, moduleDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return envDir, moduleDir
}

func TestWriteDestroyOverride_RoundTrip(t *testing.T) {
	_, moduleDir := seedWorkspaceLayout(t)

	path, err := WriteDestroyOverride(moduleDir)
	if err != nil {
		t.Fatalf("WriteDestroyOverride: %v", err)
	}
	if filepath.Base(path) != DestroyOverrideFileName {
		t.Errorf("path = %q, want basename %q", path, DestroyOverrideFileName)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read override: %v", err)
	}
	if !strings.Contains(string(data), "prevent_destroy = false") {
		t.Errorf("override content missing lifecycle override:\n%s", data)
	}

	// A stale copy from a crashed run is reclaimed by overwriting.
	if _, err := WriteDestroyOverride(moduleDir); err != nil {
		t.Fatalf("overwrite of stale override must succeed: %v", err)
	}

	if err := RemoveDestroyOverride(moduleDir); err != nil {
		t.Fatalf("RemoveDestroyOverride: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("override must be gone, stat err = %v", err)
	}
	if err := RemoveDestroyOverride(moduleDir); err != nil {
		t.Errorf("removing a missing override must succeed: %v", err)
	}
}

func TestWriteDestroyOverride_MissingModuleDir(t *testing.T) {
	if _, err := WriteDestroyOverride(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("write into a missing module dir must fail")
	}
}

func TestExecutor_StaleOverrideBlocksNonDestroy(t *testing.T) {
	envDir, moduleDir := seedWorkspaceLayout(t)
	overridePath, err := WriteDestroyOverride(moduleDir)
	if err != nil {
		t.Fatal(err)
	}
	installRecordingTerraform(t, "")
	e := New(envDir)

	calls := []struct {
		name string
		call func() error
	}{
		{"Plan", func() error { return e.Plan(context.Background(), PlanOptions{}) }},
		{"PlanStreamed", func() error { return e.PlanStreamed(context.Background(), PlanOptions{}) }},
		{"PlanDetailed", func() error { _, err := e.PlanDetailed(context.Background(), PlanOptions{}); return err }},
		{"Apply", func() error { return e.Apply(context.Background(), ApplyOptions{AutoApprove: true}) }},
	}
	for _, c := range calls {
		t.Run(c.name, func(t *testing.T) {
			err := c.call()
			var cfgErr *errtypes.ConfigError
			if !errors.As(err, &cfgErr) {
				t.Fatalf("want *errtypes.ConfigError, got %T: %v", err, err)
			}
			if !strings.Contains(err.Error(), overridePath) {
				t.Errorf("refusal must name the override path %q: %v", overridePath, err)
			}
		})
	}
	if _, err := os.Stat(filepath.Join(envDir, "argv.log")); !os.IsNotExist(err) {
		t.Error("terraform must never run while the stale override guard refuses")
	}
}

func TestExecutor_DestroySessionRunsWithOverride(t *testing.T) {
	envDir, moduleDir := seedWorkspaceLayout(t)
	if _, err := WriteDestroyOverride(moduleDir); err != nil {
		t.Fatal(err)
	}
	installRecordingTerraform(t, "")
	e := New(envDir)

	if err := e.Plan(context.Background(), PlanOptions{Destroy: true}); err != nil {
		t.Fatalf("destroy plan must run with the override present: %v", err)
	}
	if err := e.Destroy(context.Background(), DestroyOptions{UsePlan: true}); err != nil {
		t.Fatalf("destroy must run with the override present: %v", err)
	}
	lines := recordedArgv(t, envDir)
	if len(lines) != 3 {
		t.Fatalf("want destroy-plan + plan + apply argv, got %d: %v", len(lines), lines)
	}
	if !strings.HasPrefix(lines[2], "apply") {
		t.Errorf("destroy's internal apply must bypass the guard, argv: %v", lines)
	}
}

func TestWithPreventDestroyHint(t *testing.T) {
	moduleDir := "/proj/infrastructure/terraform/modules/proxmox-okd"

	t.Run("prevent_destroy stderr appends hint, preserving type", func(t *testing.T) {
		inner := &executor.ExitError{
			Command: "terraform apply", ExitCode: 1,
			Stderr: `Error: Instance cannot be destroyed ... has lifecycle.prevent_destroy set`,
		}
		err := WithPreventDestroyHint(&errtypes.ClusterError{Msg: "terraform destroy failed", Err: inner}, moduleDir)
		var clusterErr *errtypes.ClusterError
		if !errors.As(err, &clusterErr) {
			t.Fatalf("hinted error must stay *errtypes.ClusterError, got %T", err)
		}
		if !strings.Contains(err.Error(), DestroyOverridePath(moduleDir)) {
			t.Errorf("hint must name the override path: %v", err)
		}
	})

	t.Run("unrelated failure passes through unchanged", func(t *testing.T) {
		orig := &errtypes.ClusterError{
			Msg: "terraform destroy failed",
			Err: &executor.ExitError{Command: "terraform apply", ExitCode: 1, Stderr: "connection refused"},
		}
		got := WithPreventDestroyHint(orig, moduleDir)
		if !errors.Is(got, orig) || strings.Contains(got.Error(), DestroyOverrideFileName) {
			t.Errorf("unrelated error must be returned unchanged, got %v", got)
		}
	})

	t.Run("nil in nil out", func(t *testing.T) {
		if err := WithPreventDestroyHint(nil, moduleDir); err != nil {
			t.Errorf("want nil, got %v", err)
		}
	})
}
