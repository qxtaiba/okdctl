package cleanup

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// Load-bearing: terraform.tfstate must survive cleanup so destroy can still use it.
func TestCleanupTerraformEnv_PreservesState(t *testing.T) {
	dir := t.TempDir()

	writeFiles(t, dir, map[string]string{
		"terraform.tfvars":                   "vars",
		"tfplan":                             "plan",
		"destroy.tfplan":                     "dplan",
		"terraform.tfstate.backup":           "backup",
		".terraform.lock.hcl":                "lock",
		workspace.BootstrapStateSentinelFile: `{"bootstrap_enabled":false}`,
		"terraform.tfstate":                  `{"version":4,"resources":[]}`,
	})
	if err := os.MkdirAll(filepath.Join(dir, ".terraform", "providers"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := cleanupTerraformEnv(context.Background(), dir, "production", logutil.NopLogger); err != nil {
		t.Fatalf("cleanupTerraformEnv: %v", err)
	}

	mustBeGone := []string{
		"terraform.tfvars",
		"tfplan",
		"destroy.tfplan",
		".terraform.lock.hcl",
		workspace.BootstrapStateSentinelFile,
		".terraform",
	}
	for _, f := range mustBeGone {
		if _, err := os.Stat(filepath.Join(dir, f)); !os.IsNotExist(err) {
			t.Errorf("%s not removed: %v", f, err)
		}
	}

	body, err := os.ReadFile(filepath.Join(dir, "terraform.tfstate"))
	if err != nil {
		t.Fatalf("terraform.tfstate removed (DATA LOSS): %v", err)
	}
	if string(body) != `{"version":4,"resources":[]}` {
		t.Errorf("terraform.tfstate mutated: %q", body)
	}

	// terraform.tfstate.backup is the operator's rollback artefact; cleanup must not touch it.
	backup, err := os.ReadFile(filepath.Join(dir, "terraform.tfstate.backup"))
	if err != nil {
		t.Fatalf("terraform.tfstate.backup removed (DATA LOSS): %v", err)
	}
	if string(backup) != "backup" {
		t.Errorf("terraform.tfstate.backup mutated: %q", backup)
	}
}

func TestCleanupTerraformEnv_MissingDirIsNoOp(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "never-existed")
	if err := cleanupTerraformEnv(context.Background(), dir, "prod", logutil.NopLogger); err != nil {
		t.Errorf("missing dir must be nil; got %v", err)
	}
}

// Pins the invariant: terraform.tfstate must never appear in the cleanup list.
func TestTerraformFilesToRemove_DoesNotIncludeTfstate(t *testing.T) {
	if slices.Contains(terraformFilesToRemove, "terraform.tfstate") {
		t.Fatal("terraform.tfstate must not be in terraformFilesToRemove: would break destroy recoverability")
	}
}

func TestTerraform_BaseDirMissing(t *testing.T) {
	projectRoot := t.TempDir()
	err := Terraform(context.Background(), projectRoot, "", logutil.NopLogger)
	if err != nil {
		t.Errorf("missing base must be nil; got %v", err)
	}
}

// Multi-env walk (terraformEnv == "") must preserve tfstate in every env dir.
func TestTerraform_AllEnvs_PreservesEachState(t *testing.T) {
	projectRoot := t.TempDir()
	envBase := filepath.Join(projectRoot, "infrastructure", "terraform", "environments")

	envs := map[string]string{
		"production": `{"version":4,"serial":1}`,
		"staging":    `{"version":4,"serial":2}`,
	}
	for env, stateBody := range envs {
		envDir := filepath.Join(envBase, env)
		if err := os.MkdirAll(envDir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeFiles(t, envDir, map[string]string{
			"terraform.tfstate":   stateBody,
			"tfplan":              "plan",
			".terraform.lock.hcl": "lock",
		})
	}

	if err := Terraform(context.Background(), projectRoot, "", logutil.NopLogger); err != nil {
		t.Fatalf("Terraform all-envs: %v", err)
	}

	for env, stateBody := range envs {
		envDir := filepath.Join(envBase, env)

		for _, artifact := range []string{"tfplan", ".terraform.lock.hcl"} {
			if _, err := os.Stat(filepath.Join(envDir, artifact)); !os.IsNotExist(err) {
				t.Errorf("env %s: %s not removed: %v", env, artifact, err)
			}
		}

		got, err := os.ReadFile(filepath.Join(envDir, "terraform.tfstate"))
		if err != nil {
			t.Fatalf("env %s: terraform.tfstate removed (DATA LOSS): %v", env, err)
		}
		if string(got) != stateBody {
			t.Errorf("env %s: terraform.tfstate mutated: got %q, want %q", env, got, stateBody)
		}
	}
}

// EPERM on .terraform/providers must not abort the loop; tfvars still removed, tfstate survives.
func TestCleanupTerraformEnv_PartialFailureDoesNotAbort(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("requires non-root for chmod-based EPERM")
	}

	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"terraform.tfvars":         "vars",
		"terraform.tfstate":        `{"version":4,"resources":[]}`,
		"terraform.tfstate.backup": "backup",
	})

	dotTerraform := filepath.Join(dir, ".terraform")
	if err := os.MkdirAll(filepath.Join(dotTerraform, "providers"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dotTerraform, "providers", "somefile"), []byte("prov"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dotTerraform, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dotTerraform, 0o700) })

	if err := cleanupTerraformEnv(context.Background(), dir, "production", logutil.NopLogger); err != nil {
		t.Fatalf("cleanupTerraformEnv must return nil on partial failure; got %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "terraform.tfvars")); !os.IsNotExist(err) {
		t.Errorf("terraform.tfvars not removed despite being reachable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "terraform.tfstate")); err != nil {
		t.Fatalf("terraform.tfstate removed (DATA LOSS): %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "terraform.tfstate"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"version":4,"resources":[]}` {
		t.Errorf("terraform.tfstate mutated: %q", body)
	}
}

func TestTerraform_SingleEnv_PreservesState(t *testing.T) {
	projectRoot := t.TempDir()
	envDir := filepath.Dir(seedStateFile(t, projectRoot, "STATE"))
	writeFiles(t, envDir, map[string]string{"tfplan": "planned"})

	if err := Terraform(context.Background(), projectRoot, "production", logutil.NopLogger); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(envDir, "tfplan")); !os.IsNotExist(err) {
		t.Errorf("tfplan not removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(envDir, "terraform.tfstate")); err != nil {
		t.Fatalf("terraform.tfstate removed (DATA LOSS): %v", err)
	}
}
