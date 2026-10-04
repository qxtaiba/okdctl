package postinstall

import (
	"context"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

// CleanupBootstrap destroys the bootstrap VM via a scoped terraform apply
// (bootstrap_enabled=false, -target). Safe to re-run once the VM is gone.
func (p *Phase) CleanupBootstrap(ctx context.Context, cfg *config.Config, opts *Options) error {
	terraformDir := workspace.TerraformEnvDir(opts.ProjectRoot, opts.TerraformEnv)

	tf := terraform.New(terraformDir,
		terraform.WithLogger(p.Log),
		terraform.WithEnv(p.Exec.SnapshotEnv()),
	)
	defer tf.ZeroizeEnv()

	vars := map[string]string{"bootstrap_enabled": "false"}
	targets := []string{"module.okd_cluster.proxmox_virtual_environment_vm.bootstrap"}

	planFile := "bootstrap-destroy.tfplan"
	planPath := filepath.Join(terraformDir, planFile)
	// Removed on exit — a leftover plan file blocks reuse on the next run.
	defer func() {
		if err := system.SafeRemove(planPath); err != nil {
			p.Log.Warn("bootstrap: plan file cleanup failed", "err", err)
		}
	}()

	// Init and plan can both rewrite state (schema migration, refresh), so
	// both belong after the backup, alongside the apply they lead into.
	err := terraform.WithStateRecovery(ctx, tf, "bootstrap: terraform apply", func() error {
		if err := tf.Init(ctx); err != nil {
			return tf.WithLockHint(&errtypes.ClusterError{Msg: "bootstrap: terraform init failed", Err: err})
		}

		p.Log.Info("bootstrap: planning vm destruction")
		if err := tf.Plan(ctx, terraform.PlanOptions{
			OutputPlanFile: planFile,
			Vars:           vars,
			Targets:        targets,
		}); err != nil {
			return tf.WithLockHint(&errtypes.ClusterError{Msg: "bootstrap: terraform plan failed", Err: err})
		}

		// Written before apply so a crash never leaves tfvars claiming the VM
		// should exist while state says otherwise (which would trigger re-creation).
		statePath := filepath.Join(terraformDir, workspace.BootstrapStateSentinelFile)
		if err := system.AtomicWriteString(statePath, `{"bootstrap_enabled": false}`, 0o600); err != nil {
			// ClusterError not ConfigError: file is okdctl-managed, not user-authored.
			return &errtypes.ClusterError{Msg: "bootstrap: write state override", Err: err}
		}

		p.Log.Info("bootstrap: applying — destroying bootstrap vm")
		return tf.Apply(ctx, terraform.ApplyOptions{PlanFile: planPath})
	})
	if err != nil {
		return err
	}

	p.Log.Info("bootstrap: vm destroyed", "cluster", cfg.Cluster.Name)
	return nil
}
