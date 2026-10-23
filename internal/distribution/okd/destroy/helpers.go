package destroy

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/proxmox"
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

	moduleDir := workspace.TerraformModuleDir(opts.ProjectRoot)

	// Init can rewrite state during schema migration, so it belongs after the backup.
	err := terraform.WithStateRecovery(ctx, tf, "terraform destroy", func() error {
		if err := tf.Init(ctx); err != nil {
			return tf.WithLockHint(&errtypes.ClusterError{Msg: "terraform init failed", Err: err})
		}

		p.warnTopologyDrift(ctx, tf, cfg)

		// prevent_destroy on the master resource is lifted for exactly this
		// destroy via a transient module override, removed on every exit path.
		overridePath, ovrErr := terraform.WriteDestroyOverride(moduleDir)
		if ovrErr != nil {
			p.Log.Warn("destroy: could not write the transient prevent_destroy override; terraform will refuse to destroy master vms", "err", ovrErr)
		} else {
			defer func() {
				if rmErr := terraform.RemoveDestroyOverride(moduleDir); rmErr != nil {
					p.Log.Warn("destroy: could not remove the transient prevent_destroy override — delete it by hand or every non-destroy terraform run will refuse",
						"path", overridePath, "err", rmErr)
				}
			}()
		}

		p.Log.Info("terraform: destroying infrastructure", "env", opts.TerraformEnv)
		p.Log.Warn("terraform: this operation cannot be undone")

		if err := tf.Destroy(ctx, terraform.DestroyOptions{
			AutoApprove: opts.AutoApprove,
			Parallelism: opts.Parallelism,
			UsePlan:     true, // use safer plan-then-apply approach
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return terraform.WithPreventDestroyHint(err, moduleDir)
	}

	if err := tf.CleanupPlans(); err != nil {
		p.Log.Warn("terraform: plan file cleanup warning (remove stale tfplan/destroy.tfplan manually if needed)", "dir", terraformDir, "err", err)
	}

	return nil
}

// warnTopologyDrift probes the state for a master/worker instance one past
// the config's topology count, warning of a config/state mismatch without
// ever blocking the destroy.
func (p *Phase) warnTopologyDrift(ctx context.Context, tf *terraform.Executor, cfg *config.Config) {
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
		p.Log.Warn("destroy: config topology drifted from deployed state; custom iso removal may miss per-node isos beyond the config count",
			"role", string(probe.role), "config_count", probe.count)
	}
}

// customISONames returns the setup phase's per-node ISO filenames (no cluster
// prefix — removal safety relies on RemoveUnreferenced's in-use check).
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

// baseISOStorage is the stock `local` storage, where base CoreOS ISOs live.
const baseISOStorage = "local"

// removeRemoteISOs deletes the cluster's per-node ISOs from the ISO storage
// and base CoreOS ISOs from the local storage through the Proxmox API,
// skipping any a VM config still references.
func (p *Phase) removeRemoteISOs(ctx context.Context, cfg *config.Config) error {
	if p.ProxmoxCreds == nil || !p.ProxmoxCreds.IsValid() {
		return &errtypes.ConfigError{Msg: "proxmox api credentials are required for iso removal"}
	}
	px := cfg.Provider.Proxmox
	custom := customISONames(cfg)
	passes := []struct {
		label, storage string
		match          func(string) bool
	}{
		{"base coreos", baseISOStorage, nodetypes.IsCoreOSISOName},
		{"custom node", px.ISOStorage, func(name string) bool { return slices.Contains(custom, name) }},
	}
	var errs []error
	for _, pass := range passes {
		removed, inUse, err := proxmox.NewISOStore(p.ProxmoxCreds, pass.storage).RemoveUnreferenced(ctx, px.Node, pass.match)
		for _, name := range inUse {
			p.Log.Warn("iso: still referenced by a vm — skipping removal", "file", name, "storage", pass.storage)
		}
		for _, name := range removed {
			p.Log.Info("iso: removed from proxmox storage", "kind", pass.label, "file", name, "storage", pass.storage)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("remove %s isos from %s: %w", pass.label, pass.storage, err))
		}
	}
	return errors.Join(errs...)
}
