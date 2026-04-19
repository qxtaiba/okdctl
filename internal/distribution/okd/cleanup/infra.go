package cleanup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

// Terraform removes generated Terraform artifacts under
// infrastructure/terraform/environments. When terraformEnv is empty every
// environment directory is cleaned; otherwise only that one.
// terraform.tfstate is intentionally preserved so destroy can still run.
func Terraform(ctx context.Context, projectRoot, terraformEnv string, logger *slog.Logger) error {
	logger = logutil.OrNop(logger)
	logger.Info("cleanup: terraform artifacts")

	terraformBase := filepath.Join(projectRoot, "infrastructure", "terraform", "environments")

	if terraformEnv != "" {
		return cleanupTerraformEnv(ctx, filepath.Join(terraformBase, terraformEnv), terraformEnv, logger)
	}

	entries, err := os.ReadDir(terraformBase)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Info("cleanup: terraform environments directory does not exist")
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			envDir := filepath.Join(terraformBase, entry.Name())
			_ = cleanupTerraformEnv(ctx, envDir, entry.Name(), logger)
		}
	}

	return nil
}

func cleanupTerraformEnv(ctx context.Context, envDir, envName string, logger *slog.Logger) error {
	if _, err := os.Stat(envDir); os.IsNotExist(err) {
		return nil
	}

	logger = logutil.OrNop(logger)
	logger.Info(fmt.Sprintf("cleanup: terraform artifacts for environment %s", envName))

	// Note: We intentionally do NOT remove terraform.tfstate here!
	// The state file is needed to track existing resources for destroy operations.
	// Only remove it after a successful terraform destroy.
	filesToRemove := []string{
		"terraform.tfvars",
		"tfplan",
		"destroy.tfplan",
		// "terraform.tfstate" - KEEP THIS! Needed for destroy
		"terraform.tfstate.backup",
		".terraform.lock.hcl",
	}

	for _, f := range filesToRemove {
		_ = SafeRemoveWithLogger(ctx, filepath.Join(envDir, f), fmt.Sprintf("terraform %s", f), logger)
	}

	_ = SafeRemoveWithLogger(ctx, filepath.Join(envDir, ".terraform"), "terraform cache directory", logger)

	return nil
}
