package destroy

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/system"
)

// stateLockHint returns a *errtypes.ConfigError when the Terraform local
// backend lock file is present in dir, indicating a stale state lock from a
// prior crashed run. NFS / shared filesystems are the common trigger.
// Returns nil when the lock file is absent. The caller never auto-unlocks —
// the message names the lock so the operator can run terraform force-unlock
// after confirming no live process is holding it.
func stateLockHint(dir string) error {
	lockFile := filepath.Join(dir, ".terraform.tfstate.lock.info")
	if !system.FileExists(lockFile) {
		return nil
	}
	return &errtypes.ConfigError{
		Msg: fmt.Sprintf(
			"terraform state locked at %s — run 'terraform force-unlock <id>' in %s after confirming no other okdctl run is active",
			lockFile, dir,
		),
	}
}

func (p *Phase) destroyInfrastructure(ctx context.Context, opts *Options) error {
	terraformDir := filepath.Join(opts.ProjectRoot, "infrastructure", "terraform", "environments", opts.TerraformEnv)

	if !system.DirExists(terraformDir) {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("terraform environment directory not found: %s", terraformDir)}
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
		if hint := stateLockHint(terraformDir); hint != nil {
			return hint
		}
		return &errtypes.ClusterError{Msg: "terraform init failed", Err: err}
	}

	p.Log.Info(fmt.Sprintf("terraform: destroying infrastructure in %s", opts.TerraformEnv))
	p.Log.Warn("terraform: this operation cannot be undone")

	if err := tf.Destroy(ctx, terraform.DestroyOptions{
		AutoApprove: opts.AutoApprove,
		Parallelism: opts.Parallelism,
		UsePlan:     true, // use safer plan-then-apply approach
	}); err != nil {
		return &errtypes.ClusterError{Msg: "terraform destroy failed", Err: err}
	}

	if err := tf.CleanupPlans(); err != nil {
		p.Log.Warn("terraform: plan file cleanup warning (remove stale tfplan/destroy.tfplan manually if needed)", "dir", terraformDir, "err", err)
	}

	return nil
}
