package postinstall

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/system"
)

// CleanupBootstrap destroys the bootstrap VM by re-applying terraform with bootstrap_enabled=false.
// Uses -target to scope the operation to the bootstrap resource only, preventing
// unintended side effects on other resources (e.g., workers being shut down).
// Re-running after the VM is already gone is a no-op: terraform reports zero
// changes and apply succeeds cleanly.
func (p *Phase) CleanupBootstrap(ctx context.Context, cfg *config.Config, opts *Options) error {
	terraformDir := filepath.Join(opts.ProjectRoot, "infrastructure", "terraform", "environments", opts.TerraformEnv)

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
	// Always sweep the plan file at function exit. Without this a plan
	// or apply error left the .tfplan in place; the next bootstrap-destroy
	// run would refuse to overwrite it or re-use stale targets.
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

	// Write the sentinel before apply so a crash between apply-success and
	// file-write cannot leave the VM destroyed but tfvars still claiming
	// bootstrap_enabled=true (which would trigger re-creation on the next plan).
	// If apply fails, the sentinel is harmless: terraform state still tracks the
	// VM as present, so the next plan is a correct retry.
	statePath := filepath.Join(terraformDir, phase.BootstrapStateSentinelFile)
	if err := system.AtomicWriteString(statePath, `{"bootstrap_enabled": false}`, 0o600); err != nil {
		// State-write during cluster lifecycle → ClusterError (exit 4), not
		// ConfigError; bootstrap-state.auto.tfvars.json is managed by okdctl, not the user.
		return &errtypes.ClusterError{Msg: "bootstrap: failed to write state override", Err: err}
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
