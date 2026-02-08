package postinstall

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/infrastructure/terraform"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

// CleanupBootstrap destroys the bootstrap VM by re-applying terraform with bootstrap_enabled=false.
// The -var flag overrides the value in terraform.tfvars without modifying the file.
func (p *Phase) CleanupBootstrap(ctx context.Context, cfg *config.Config, opts Options) error {
	terraformDir := filepath.Join(opts.ProjectRoot, "infrastructure", "terraform", "environments", opts.TerraformEnv)
	tfvarsFile := filepath.Join(terraformDir, "terraform.tfvars")

	if !system.FileExists(tfvarsFile) {
		p.Log.Warn("bootstrap: terraform.tfvars not found — skipping bootstrap cleanup")
		return nil
	}

	tf := terraform.New(terraformDir,
		terraform.WithLogger(p.Log),
		terraform.WithEnv(p.Exec.Env),
	)

	if err := tf.Init(ctx); err != nil {
		return utils.WrapError("bootstrap: terraform init failed", err)
	}

	vars := map[string]string{"bootstrap_enabled": "false"}

	p.Log.Info("bootstrap: planning vm destruction")
	planFile := "bootstrap-destroy.tfplan"
	if err := tf.Plan(ctx, terraform.PlanOptions{
		OutputPlanFile: planFile,
		Vars:           vars,
	}); err != nil {
		return utils.WrapError("bootstrap: terraform plan failed", err)
	}

	p.Log.Info("bootstrap: applying — destroying bootstrap vm")
	if err := tf.Apply(ctx, terraform.ApplyOptions{
		PlanFile: filepath.Join(terraformDir, planFile),
	}); err != nil {
		return utils.WrapError("bootstrap: terraform apply failed", err)
	}

	// Clean up plan file.
	_ = system.SafeRemove(filepath.Join(terraformDir, planFile))

	p.Log.Info(fmt.Sprintf("bootstrap: vm destroyed (no longer needed for %s)", cfg.Cluster.Name))
	return nil
}
