package install

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/infrastructure/terraform"
)

// StartWorkerVMs starts worker VMs after bootstrap completes so they can reach the MCS.
func (p *Phase) StartWorkerVMs(ctx context.Context, cfg *config.Config, opts *Options) error {
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
		return fmt.Errorf("terraform init failed: %w", err)
	}

	applyOpts := terraform.ApplyOptions{
		AutoApprove: true,
		Vars: map[string]string{
			"start_workers_immediately": "true",
		},
	}

	if err := tf.Apply(ctx, applyOpts); err != nil {
		return fmt.Errorf("failed to start worker VMs: %w", err)
	}

	p.Log.Info("workers: all worker nodes started successfully")
	return nil
}
