package postinstall

import (
	"context"
	"fmt"
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

	if err := tf.Init(ctx); err != nil {
		return tf.WithLockHint(&errtypes.ClusterError{Msg: "bootstrap: terraform init failed", Err: err})
	}

	vars := map[string]string{"bootstrap_enabled": "false"}
	targets := []string{"module.okd_cluster.proxmox_virtual_environment_vm.bootstrap"}

	p.Log.Info("bootstrap: planning vm destruction")
	planFile := "bootstrap-destroy.tfplan"
	planPath := filepath.Join(terraformDir, planFile)
	// Removed on exit — a leftover plan file blocks reuse on the next run.
	defer func() {
		if err := system.SafeRemove(planPath); err != nil {
			p.Log.Warn("bootstrap: plan file cleanup failed", "err", err)
		}
	}()
	if err := tf.Plan(ctx, terraform.PlanOptions{
		OutputPlanFile: planFile,
		Vars:           vars,
		Targets:        targets,
	}); err != nil {
		return tf.WithLockHint(&errtypes.ClusterError{Msg: "bootstrap: terraform plan failed", Err: err})
	}

	snapPath, snapErr := tf.SnapshotState(ctx)
	if snapErr != nil {
		return &errtypes.ClusterError{Msg: "bootstrap: state snapshot failed", Err: snapErr}
	}

	// Written before apply so a crash never leaves tfvars claiming the VM
	// should exist while state says otherwise (which would trigger re-creation).
	statePath := filepath.Join(terraformDir, workspace.BootstrapStateSentinelFile)
	if err := system.AtomicWriteString(statePath, `{"bootstrap_enabled": false}`, 0o600); err != nil {
		// ClusterError not ConfigError: file is okdctl-managed, not user-authored.
		return &errtypes.ClusterError{Msg: "bootstrap: write state override", Err: err}
	}

	p.Log.Info("bootstrap: applying — destroying bootstrap vm")
	if err := tf.Apply(ctx, terraform.ApplyOptions{
		PlanFile: planPath,
	}); err != nil {
		msg := "bootstrap: terraform apply failed"
		if snapPath != "" {
			msg = fmt.Sprintf("bootstrap: terraform apply failed (state backup: %s)", snapPath)
		}
		return tf.WithLockHint(&errtypes.ClusterError{Msg: msg, Err: err})
	}

	p.Log.Info("bootstrap: vm destroyed", "cluster", cfg.Cluster.Name)
	return nil
}
