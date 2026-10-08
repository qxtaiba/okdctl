package provision

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/proxmox"
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

const (
	privAllocateTemplate = "Datastore.AllocateTemplate"
	privAudit            = "Datastore.Audit"
	privAllocateSpace    = "Datastore.AllocateSpace"
	privAllocate         = "Datastore.Allocate"
)

// sharedISOStorageTypes are the storage types whose ISO directory is the same
// filesystem on every node; a directory storage's shared flag only asserts that.
var sharedISOStorageTypes = []string{"nfs", "cifs", "cephfs"}

// ValidateISOStorage is the pre-mutation ISO storage check: the credentials
// must hold the upload privileges and every multi-host destination must
// share the ISO storage.
func (p *Provisioner) ValidateISOStorage(ctx context.Context, cfg *config.Config) error {
	if cfg.Provider.Proxmox == nil {
		return nil
	}
	if err := p.checkISOPrivileges(ctx, cfg); err != nil {
		return err
	}
	return p.validateISOPlacement(ctx, cfg)
}

// checkISOPrivileges fails only on a privilege the listing proves missing;
// absent credentials or an unreadable listing defer to the upload step.
func (p *Provisioner) checkISOPrivileges(ctx context.Context, cfg *config.Config) error {
	if p.ProxmoxCreds == nil || !p.ProxmoxCreds.IsValid() {
		return nil
	}
	storage := cfg.Provider.Proxmox.ISOStorage
	held, err := proxmox.NewISOStore(p.ProxmoxCreds, storage).Privileges(ctx)
	if err != nil {
		p.Log.Warn("iso: could not list proxmox privileges; the upload will report any gap", "err", err)
		return nil
	}
	var missing []string
	if !held[privAllocateTemplate] {
		missing = append(missing, privAllocateTemplate)
	}
	if !held[privAudit] && !held[privAllocateSpace] {
		missing = append(missing, privAudit)
	}
	if len(missing) > 0 {
		return (&errtypes.ConfigError{
			Msg: fmt.Sprintf("proxmox credentials lack %s on /storage/%s, needed to upload node isos", strings.Join(missing, ", "), storage),
		}).WithHint("grant a role with these privileges on /storage/" + storage + " to the user or api token")
	}
	if !held[privAllocate] {
		p.Log.Warn("iso: proxmox credentials lack Datastore.Allocate; okdctl destroy will not be able to remove the node isos", "path", "/storage/"+storage)
	}
	return nil
}

func (p *Provisioner) validateISOPlacement(ctx context.Context, cfg *config.Config) error {
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
