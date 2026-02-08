package install

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/infrastructure/terraform"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

// StartWorkerVMs starts worker VMs via terraform apply with started=true.
// This is called after bootstrap completes to ensure workers can reach the MCS.
func (p *Phase) StartWorkerVMs(ctx context.Context, cfg *config.Config, opts Options) error {
	if cfg.Topology.Workers.Count == 0 {
		p.Log.Info("workers: no workers configured, skipping")
		return nil
	}

	p.Log.Info(fmt.Sprintf("workers: starting %d worker nodes", cfg.Topology.Workers.Count))

	terraformDir := filepath.Join(opts.ProjectRoot, "infrastructure", "terraform", "environments", opts.TerraformEnv)

	tf := terraform.NewWithVarFile(
		terraformDir,
		filepath.Join(terraformDir, "terraform.tfvars"),
		terraform.WithLogger(p.Log),
		terraform.WithEnv(p.Exec.Env),
	)

	if err := tf.Init(ctx); err != nil {
		return utils.WrapError("terraform init failed", err)
	}

	applyOpts := terraform.ApplyOptions{
		AutoApprove: true,
		Vars: map[string]string{
			"start_workers_immediately": "true",
		},
	}

	if err := tf.Apply(ctx, applyOpts); err != nil {
		return utils.WrapError("failed to start worker VMs", err)
	}

	p.Log.Info("workers: all worker nodes started successfully")
	return nil
}
