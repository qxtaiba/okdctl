package destroy

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/qxtaiba/okd-proxmox-cli/internal/infrastructure/terraform"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

// destroyInfrastructure runs Terraform destroy using the terraform executor.
func (p *Phase) destroyInfrastructure(ctx context.Context, opts Options) error {
	terraformDir := filepath.Join(opts.ProjectRoot, "infrastructure", "terraform", "environments", opts.TerraformEnv)

	if !system.DirExists(terraformDir) {
		return fmt.Errorf("terraform environment directory not found: %s", terraformDir)
	}

	tf := terraform.New(terraformDir,
		terraform.WithLogger(p.Log),
		terraform.WithVerbose(opts.Debug),
		terraform.WithEnv(p.Exec.Env),
	)

	if !tf.HasState() {
		p.Log.Warn("terraform: no state file found - infrastructure may already be destroyed")
		return nil
	}

	if err := tf.Init(ctx); err != nil {
		return utils.WrapError("terraform init failed", err)
	}

	p.Log.Info(fmt.Sprintf("terraform: destroying infrastructure in %s", opts.TerraformEnv))
	p.Log.Warn("terraform: this operation cannot be undone")

	if err := tf.Destroy(ctx, terraform.DestroyOptions{
		AutoApprove: opts.AutoApprove,
		Parallelism: opts.Parallelism,
		UsePlan:     true, // use safer plan-then-apply approach
	}); err != nil {
		return utils.WrapError("terraform destroy failed", err)
	}

	if err := tf.Cleanup(); err != nil {
		p.Log.Warn(fmt.Sprintf("terraform: plan file cleanup warning: %v", err))
	}

	return nil
}
