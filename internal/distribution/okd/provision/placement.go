package provision

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

func placementNodes(cfg *config.Config) []string {
	nodes := []string{nodetypes.ProxmoxNode(cfg, nodetypes.RoleBootstrap, 0)}
	for role, count := range map[nodetypes.NodeRole]int{nodetypes.RoleMaster: cfg.Topology.ControlPlane.Count, nodetypes.RoleWorker: cfg.Topology.Workers.Count} {
		for i := range count {
			nodes = append(nodes, nodetypes.ProxmoxNode(cfg, role, i))
		}
	}
	slices.Sort(nodes)
	return slices.Compact(nodes)
}

// sharedISOStorageTypes are the storage types whose ISO directory is the same
// filesystem on every node; a directory storage's shared flag only asserts that.
var sharedISOStorageTypes = []string{"nfs", "cifs", "cephfs"}

// ValidateISOPlacement requires shared ISO storage for every multi-host destination before mutation.
func (p *Provisioner) ValidateISOPlacement(ctx context.Context, cfg *config.Config) error {
	if cfg.Provider.Proxmox == nil || len(placementNodes(cfg)) < 2 {
		return nil
	}
	store, err := p.isoStore(cfg)
	if err != nil {
		return err
	}
	storage := cfg.Provider.Proxmox.ISOStorage
	for _, node := range placementNodes(cfg) {
		status, err := store.Status(ctx, node)
		if err != nil {
			return fmt.Errorf("check iso storage visibility: %w", err)
		}
		if !slices.Contains(sharedISOStorageTypes, status.Type) || !slices.Contains(status.Content, "iso") {
			return fmt.Errorf("multi-host placement requires nfs, cifs, or cephfs ISO storage %q with ISO content enabled", storage)
		}
		if !status.Active || !status.Enabled {
			return fmt.Errorf("ISO storage %s is unavailable on %s", storage, node)
		}
	}
	return nil
}

func (p *Provisioner) verifySharedISOs(ctx context.Context, cfg *config.Config, files []string) error {
	if len(placementNodes(cfg)) < 2 {
		return nil
	}
	store, err := p.isoStore(cfg)
	if err != nil {
		return err
	}
	for _, node := range placementNodes(cfg) {
		volumes, err := store.Volumes(ctx, node)
		if err != nil {
			return fmt.Errorf("check iso visibility: %w", err)
		}
		for _, file := range files {
			if _, ok := volumes[filepath.Base(file)]; !ok {
				return fmt.Errorf("ISO %s is not visible on %s", store.VolumeID(filepath.Base(file)), node)
			}
		}
	}
	return nil
}
