package destroy

import (
	"context"
	"fmt"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

func (p *Phase) destroyInfrastructure(ctx context.Context, cfg *config.Config, opts *Options) error {
	terraformDir := workspace.TerraformEnvDir(opts.ProjectRoot, opts.TerraformEnv)

	if !system.DirExists(terraformDir) {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("terraform environment directory not found: %s", terraformDir)}
	}

	tf := terraform.New(
		terraformDir,
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

	p.warnTopologyDrift(ctx, tf, cfg, len(opts.TerraformTargets) > 0)

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

// warnTopologyDrift probes the state for a master/worker instance one past
// the config's topology count. A hit means the config was edited since
// deploy: scoped --only destroys expand targets from config counts and
// would leave the higher-index VMs running, and customISONames would miss
// their per-node ISOs. Best-effort — a failed probe warns and never blocks
// the destroy, which tears down whatever the state actually holds.
func (p *Phase) warnTopologyDrift(ctx context.Context, tf *terraform.Executor, cfg *config.Config, scoped bool) {
	probes := []struct {
		role  nodetypes.NodeRole
		count int
	}{
		{nodetypes.RoleMaster, cfg.Topology.ControlPlane.Count},
		{nodetypes.RoleWorker, cfg.Topology.Workers.Count},
	}
	for _, probe := range probes {
		addr := fmt.Sprintf("module.okd_cluster.proxmox_virtual_environment_vm.%s[%d]", probe.role, probe.count)
		present, err := tf.StateHasResource(ctx, addr)
		if err != nil {
			p.Log.Warn("destroy: topology drift probe failed; cannot verify config counts against deployed state",
				"addr", addr, "err", err)
			continue
		}
		if !present {
			continue
		}
		if scoped {
			p.Log.Warn("destroy: config topology drifted from deployed state; a scoped destroy expanded from config counts leaves higher-index vms running — run an unscoped destroy to remove them",
				"role", string(probe.role), "config_count", probe.count)
		} else {
			p.Log.Warn("destroy: config topology drifted from deployed state; custom iso removal may miss per-node isos beyond the config count",
				"role", string(probe.role), "config_count", probe.count)
		}
	}
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
