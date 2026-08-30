package cleanup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

// Terraform removes generated artifacts under infrastructure/terraform/environments
// (all envs if terraformEnv is empty); terraform.tfstate is preserved so destroy can still run.
func Terraform(ctx context.Context, projectRoot, terraformEnv string, logger *slog.Logger) error {
	logger = logutil.OrNop(logger)
	logger.Info("cleanup: terraform artifacts")

	terraformBase := workspace.TerraformEnvDir(projectRoot, "")

	if terraformEnv != "" {
		return cleanupTerraformEnv(ctx, filepath.Join(terraformBase, terraformEnv), terraformEnv, logger)
	}

	entries, err := os.ReadDir(terraformBase)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			logger.Info("cleanup: terraform environments directory does not exist")
			return nil
		}
		return &errtypes.ConfigError{Msg: "read terraform environments directory", Err: err}
	}

	for _, entry := range entries {
		if entry.IsDir() {
			envDir := filepath.Join(terraformBase, entry.Name())
			_ = cleanupTerraformEnv(ctx, envDir, entry.Name(), logger)
		}
	}

	return nil
}

// terraformFilesToRemove lists cleanup targets; terraform.tfstate is
// deliberately absent so destroy can still run.
var terraformFilesToRemove = []string{
	"terraform.tfvars",
	"tfplan",
	"destroy.tfplan",
	".terraform.lock.hcl",
	workspace.BootstrapStateSentinelFile,
}

// terraformCleanupDone reports whether every terraform-cleanup artifact is
// already absent (else a prior run was partial).
func terraformCleanupDone(opts *Options) (bool, error) {
	if opts.TerraformEnv == "" {
		return false, nil
	}
	envDir := workspace.TerraformEnvDir(opts.ProjectRoot, opts.TerraformEnv)
	for _, name := range terraformFilesToRemove {
		if system.FileExists(filepath.Join(envDir, name)) {
			return false, nil
		}
	}
	if system.DirExists(filepath.Join(envDir, ".terraform")) {
		return false, nil
	}
	if opts.PostDestroy && system.FileExists(filepath.Join(envDir, "terraform.tfstate")) {
		return false, nil
	}
	return true, nil
}

func cleanupTerraformEnv(ctx context.Context, envDir, envName string, logger *slog.Logger) error {
	if _, err := os.Stat(envDir); errors.Is(err, os.ErrNotExist) {
		return nil
	}

	logger = logutil.OrNop(logger)
	logger.Info("cleanup: terraform artifacts for environment", "env", envName)

	for _, f := range terraformFilesToRemove {
		_ = SafeRemoveWithLogger(ctx, filepath.Join(envDir, f), fmt.Sprintf("terraform %s", f), logger)
	}

	_ = SafeRemoveWithLogger(ctx, filepath.Join(envDir, ".terraform"), "terraform cache directory", logger)

	return nil
}
