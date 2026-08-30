package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTerraformEnvDirCheckResolvesAgainstProjectRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "infrastructure", "terraform", "environments", "staging"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Runs from a subdirectory so the check must follow the resolved root, not cwd.
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)

	cfg := DefaultConfig()
	cfg.Deployment.TerraformEnv = "staging"

	withRoot := ValidateWithOptions(cfg, ValidationOptions{Scope: ScopeEnums, ProjectRoot: root})
	if hasFieldError(withRoot, FieldDeploymentTerraformEnv) {
		t.Errorf("ProjectRoot-anchored validation flagged an existing environment: %v", withRoot.Errors)
	}

	cwdRelative := ValidateWithOptions(cfg, ValidationOptions{Scope: ScopeEnums})
	if !hasFieldError(cwdRelative, FieldDeploymentTerraformEnv) {
		t.Error("cwd-relative validation from a subdirectory found the environment; expected a miss proving the ProjectRoot anchor matters")
	}
}
