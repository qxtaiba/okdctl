package cleanup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/system"
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
		return &errtypes.ConfigError{Msg: "failed to read terraform environments directory", Err: err}
	}

	for _, entry := range entries {
		if entry.IsDir() {
			envDir := filepath.Join(terraformBase, entry.Name())
			_ = cleanupTerraformEnv(ctx, envDir, entry.Name(), logger)
		}
	}

	return nil
}

// terraformFilesToRemove lists the generated Terraform artefacts that cleanup
// may delete. terraform.tfstate is intentionally absent: it must survive so
// that destroy can still run against existing infrastructure resources.
var terraformFilesToRemove = []string{
	"terraform.tfvars",
	"tfplan",
	"destroy.tfplan",
	".terraform.lock.hcl",
	phase.BootstrapStateSentinelFile,
}

// terraformCleanupDone reports whether every artifact terraform cleanup
// removes is already absent. A single present artifact means a prior run
// was partial and cleanup must resume.
func terraformCleanupDone(opts *Options) (bool, error) {
	if opts.TerraformEnv == "" {
		return false, nil
	}
	envDir := filepath.Join(opts.ProjectRoot, "infrastructure", "terraform", "environments", opts.TerraformEnv)
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
	if _, err := os.Stat(envDir); os.IsNotExist(err) {
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
