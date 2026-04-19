package postinstall

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/system"
)

// CleanupBootstrap destroys the bootstrap VM by re-applying terraform with bootstrap_enabled=false.
// Uses -target to scope the operation to the bootstrap resource only, preventing
// unintended side effects on other resources (e.g., workers being shut down).
func (p *Phase) CleanupBootstrap(ctx context.Context, cfg *config.Config, opts *Options) error {
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
		return &errtypes.ClusterError{Msg: "bootstrap: terraform init failed", Err: err}
	}

	vars := map[string]string{"bootstrap_enabled": "false"}
	targets := []string{"module.okd_cluster.proxmox_virtual_environment_vm.bootstrap"}

	p.Log.Info("bootstrap: planning vm destruction")
	planFile := "bootstrap-destroy.tfplan"
	if err := tf.Plan(ctx, terraform.PlanOptions{
		OutputPlanFile: planFile,
		Vars:           vars,
		Targets:        targets,
	}); err != nil {
		return &errtypes.ClusterError{Msg: "bootstrap: terraform plan failed", Err: err}
	}

	p.Log.Info("bootstrap: applying — destroying bootstrap vm")
	if err := tf.Apply(ctx, terraform.ApplyOptions{
		PlanFile: filepath.Join(terraformDir, planFile),
	}); err != nil {
		return &errtypes.ClusterError{Msg: "bootstrap: terraform apply failed", Err: err}
	}

	// Clean up plan file.
	if err := system.SafeRemove(filepath.Join(terraformDir, planFile)); err != nil {
		p.Log.Warn("bootstrap: plan file cleanup failed", "err", err)
	}

	p.Log.Info(fmt.Sprintf("bootstrap: vm destroyed (no longer needed for %s)", cfg.Cluster.Name))
	return nil
}
