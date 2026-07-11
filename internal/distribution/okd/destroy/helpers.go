package destroy

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/system"
)

func (p *Phase) destroyInfrastructure(ctx context.Context, opts *Options) error {
	terraformDir := filepath.Join(opts.ProjectRoot, "infrastructure", "terraform", "environments", opts.TerraformEnv)

	if !system.DirExists(terraformDir) {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("terraform environment directory not found: %s", terraformDir)}
	}

	tf := terraform.New(terraformDir,
		terraform.WithLogger(p.Log),
		terraform.WithEnv(p.Exec.SnapshotEnv()),
	)
	defer tf.ZeroizeEnv()

	switch tf.StateStatus() {
	case terraform.StateStatusMissing, terraform.StateStatusEmpty:
		p.Log.Warn("terraform: no state file found - infrastructure may already be destroyed")
		return nil
	case terraform.StateStatusCorrupt:
		msg := "terraform state is corrupt; restore the state file and re-run okdctl destroy"
		if bak := tf.NewestBakSnapshot(); bak != "" {
			msg = fmt.Sprintf("terraform state is corrupt (newest backup: %s); restore and re-run okdctl destroy", bak)
		}
		return &errtypes.ClusterError{Msg: msg}
	}

	// Snapshot before Init: terraform init may rewrite terraform_version on schema
	// migration, making a post-init snapshot useless as a pre-run restore point.
	snapPath, snapErr := tf.SnapshotState(ctx)
	if snapErr != nil {
		return &errtypes.ClusterError{Msg: "terraform destroy: state snapshot failed", Err: snapErr}
	}

	if err := tf.Init(ctx); err != nil {
		return tf.WithLockHint(&errtypes.ClusterError{Msg: "terraform init failed", Err: err})
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
		return tf.WithLockHint(&errtypes.ClusterError{Msg: msg, Err: err})
	}

	if err := tf.CleanupPlans(); err != nil {
		p.Log.Warn("terraform: plan file cleanup warning (remove stale tfplan/destroy.tfplan manually if needed)", "dir", terraformDir, "err", err)
	}

	return nil
}

// customISONames returns the exact per-node custom ISO filenames the setup
// phase uploads to the Proxmox host (bootstrap.iso, master<N>.iso,
// worker<N>.iso) for cfg's topology — see setup.BuildNodeList for the
// naming this mirrors. Names carry no cluster prefix; removal-side safety
// comes from hostssh.RemoveCustomISOsFromProxmox's in-use check, not the name.
func customISONames(cfg *config.Config) []string {
	names := []string{string(nodetypes.RoleBootstrap) + ".iso"}
	for i := range cfg.Topology.ControlPlane.Count {
		names = append(names, fmt.Sprintf("%s%d.iso", nodetypes.RoleMaster, i))
	}
	for i := range cfg.Topology.Workers.Count {
		names = append(names, fmt.Sprintf("%s%d.iso", nodetypes.RoleWorker, i))
	}
	return names
}
