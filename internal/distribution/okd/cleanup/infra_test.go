package cleanup

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

// TestCleanupTerraformEnv_PreservesState is the load-bearing test for the
// invariant "terraform.tfstate must survive a cleanup run so destroy can
// still use it." Regressing this loses cluster state.
func TestCleanupTerraformEnv_PreservesState(t *testing.T) {
	dir := t.TempDir()

	// Seed an env dir with every file cleanupTerraformEnv is expected to
	// remove PLUS terraform.tfstate, which it must NOT touch.
	files := map[string]string{
		"terraform.tfvars":        "vars",
		"tfplan":                  "plan",
		"destroy.tfplan":          "dplan",
		"terraform.tfstate.backup": "backup",
		".terraform.lock.hcl":     "lock",
		"terraform.tfstate":       `{"version":4,"resources":[]}`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
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
		"terraform.tfstate.backup",
		".terraform.lock.hcl",
		".terraform",
	}
	for _, f := range mustBeGone {
		if _, err := os.Stat(filepath.Join(dir, f)); !os.IsNotExist(err) {
			t.Errorf("%s not removed: %v", f, err)
		}
	}

	// THE invariant: terraform.tfstate must still be present with original
	// contents intact.
	body, err := os.ReadFile(filepath.Join(dir, "terraform.tfstate"))
	if err != nil {
		t.Fatalf("terraform.tfstate removed (DATA LOSS): %v", err)
	}
	if string(body) != `{"version":4,"resources":[]}` {
		t.Errorf("terraform.tfstate mutated: %q", body)
	}
}

func TestCleanupTerraformEnv_MissingDirIsNoOp(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "never-existed")
	if err := cleanupTerraformEnv(context.Background(), dir, "prod", logutil.NopLogger); err != nil {
		t.Errorf("missing dir must be nil; got %v", err)
	}
}

// TestTerraform_BaseDirMissing exercises the public Terraform entry point
// when the environments/ base does not exist — should log and return nil,
// not error.
func TestTerraform_BaseDirMissing(t *testing.T) {
	projectRoot := t.TempDir() // no infrastructure/terraform/environments under this
	err := Terraform(context.Background(), projectRoot, "", logutil.NopLogger)
	if err != nil {
		t.Errorf("missing base must be nil; got %v", err)
	}
}

// TestTerraform_SingleEnv_PreservesState combines the Terraform() entry
// point with the tfstate-preservation contract.
func TestTerraform_SingleEnv_PreservesState(t *testing.T) {
	projectRoot := t.TempDir()
	envDir := filepath.Join(projectRoot, "infrastructure", "terraform", "environments", "production")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(envDir, "terraform.tfstate"), []byte("STATE"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(envDir, "tfplan"), []byte("planned"), 0o600); err != nil {
		t.Fatal(err)
	}

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
