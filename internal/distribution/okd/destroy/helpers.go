package destroy

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/system"
)

func (p *Phase) destroyInfrastructure(ctx context.Context, opts *Options) error {
	terraformDir := filepath.Join(opts.ProjectRoot, "infrastructure", "terraform", "environments", opts.TerraformEnv)

	if !system.DirExists(terraformDir) {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("terraform environment directory not found: %s", terraformDir)}
	}

	tf := terraform.New(terraformDir,
		terraform.WithLogger(p.Log),
		terraform.WithVerbose(opts.Debug),
		terraform.WithEnv(p.Exec.SnapshotEnv()),
	)
	defer tf.ZeroizeEnv()

	if !tf.HasState() {
		p.Log.Warn("terraform: no state file found - infrastructure may already be destroyed")
		return nil
	}

	if err := tf.Init(ctx); err != nil {
		if hint := tf.LockHint(); hint != nil {
			return errors.Join(hint, &errtypes.ClusterError{Msg: "terraform init failed", Err: err})
		}
		return &errtypes.ClusterError{Msg: "terraform init failed", Err: err}
	}

	snapPath, snapErr := tf.SnapshotState(ctx)
	if snapErr != nil {
		return &errtypes.ClusterError{Msg: "terraform destroy: state snapshot failed", Err: snapErr}
	}

	p.Log.Info("terraform: destroying infrastructure", "env", opts.TerraformEnv)
	p.Log.Warn("terraform: this operation cannot be undone")

	if err := tf.Destroy(ctx, terraform.DestroyOptions{
		AutoApprove: opts.AutoApprove,
		Parallelism: opts.Parallelism,
		Targets:     opts.TerraformTargets,
		UsePlan:     true, // use safer plan-then-apply approach
	}); err != nil {
		msg := "terraform destroy failed"
		if snapPath != "" {
			msg = fmt.Sprintf("terraform destroy failed (state backup: %s)", snapPath)
		}
		return &errtypes.ClusterError{Msg: msg, Err: err}
	}

	if err := tf.CleanupPlans(); err != nil {
		p.Log.Warn("terraform: plan file cleanup warning (remove stale tfplan/destroy.tfplan manually if needed)", "dir", terraformDir, "err", err)
	}

	return nil
}
