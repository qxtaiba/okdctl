package install

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
)

// StartWorkerVMs starts worker VMs after bootstrap completes so they can reach the MCS.
func (p *Phase) StartWorkerVMs(ctx context.Context, cfg *config.Config, opts *Options) error {
	if cfg.Topology.Workers.Count == 0 {
		p.Log.Info("workers: no workers configured, skipping")
		return nil
	}

	p.Log.Info(fmt.Sprintf("workers: starting %d worker nodes", cfg.Topology.Workers.Count))

	terraformDir := filepath.Join(opts.ProjectRoot, "infrastructure", "terraform", "environments", opts.TerraformEnv)

	tf := terraform.New(
		terraformDir,
		terraform.WithLogger(p.Log),
		terraform.WithEnv(p.Exec.Env),
	)

	if err := tf.Init(ctx); err != nil {
		return &errtypes.ClusterError{Msg: "terraform init failed", Err: err}
	}

	applyOpts := terraform.ApplyOptions{
		AutoApprove: true,
		Vars: map[string]string{
			"start_workers_immediately": "true",
		},
		// Without scoping, this apply reconciles the full state — a stray
		// tfvars edit elsewhere would be silently applied alongside the
		// worker start. Bootstrap takes the same precaution.
		Targets: []string{"module.okd_cluster.proxmox_virtual_environment_vm.worker"},
	}

	if err := tf.Apply(ctx, applyOpts); err != nil {
		return &errtypes.ClusterError{Msg: "failed to start worker VMs", Err: err}
	}

	p.Log.Info("workers: all worker nodes started successfully")
	return nil
}
