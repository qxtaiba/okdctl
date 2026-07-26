package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTerraformEnvName(t *testing.T) {
	if got := (&Config{}).TerraformEnvName(); got != "production" {
		t.Errorf("TerraformEnvName() = %q; want production default", got)
	}
	cfg := &Config{Deployment: DeploymentConfig{TerraformEnv: "staging"}}
	if got := cfg.TerraformEnvName(); got != "staging" {
		t.Errorf("TerraformEnvName() = %q; want staging", got)
	}
}

func TestTerraformEnvDirCheckResolvesAgainstProjectRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "infrastructure", "terraform", "environments", "staging"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Invoke from a subdirectory of the workspace: the check must follow the
	// resolved root, not the process cwd.
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)

	cfg := DefaultConfig()
	cfg.Deployment.TerraformEnv = "staging"

	hasEnvError := func(r *ValidationResult) bool {
		for _, e := range r.Errors {
			if e.Field == FieldDeploymentTerraformEnv {
				return true
			}
		}
		return false
	}

	withRoot := ValidateWithOptions(cfg, ValidationOptions{Scope: ScopeEnums, ProjectRoot: root})
	if hasEnvError(withRoot) {
		t.Errorf("ProjectRoot-anchored validation flagged an existing environment: %v", withRoot.Errors)
	}

	cwdRelative := ValidateWithOptions(cfg, ValidationOptions{Scope: ScopeEnums})
	if !hasEnvError(cwdRelative) {
		t.Error("cwd-relative validation from a subdirectory found the environment; expected a miss proving the ProjectRoot anchor matters")
	}
}
