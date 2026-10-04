package provision

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/infrastructure/proxmox/hostssh"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/sshpin"
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

// ValidateISOPlacement requires shared ISO storage for every multi-host destination before mutation.
func (p *Provisioner) ValidateISOPlacement(ctx context.Context, cfg *config.Config) error {
	if cfg.Provider.Proxmox == nil || len(placementNodes(cfg)) < 2 {
		return nil
	}
	px := cfg.Provider.Proxmox
	host := hostssh.ProxmoxBareHost(px.Host)
	known, err := sshpin.Verify(ctx, host, px.SSHHostFingerprint, px.RequirePinnedFingerprint, p.Log)
	if err != nil {
		return err
	}
	if known != "" {
		defer os.Remove(known)
	}
	var storage struct {
		Type    string `json:"type"`
		Content string `json:"content"`
	}
	if err := p.storageQuery(ctx, host, known, "/storage/"+px.ISOStorage, &storage); err != nil {
		return err
	}
	if !slices.Contains([]string{"nfs", "cifs", "cephfs"}, storage.Type) || !slices.Contains(strings.Split(storage.Content, ","), "iso") {
		return fmt.Errorf("multi-host placement requires nfs, cifs, or cephfs ISO storage %q with ISO content enabled", px.ISOStorage)
	}
	for _, node := range placementNodes(cfg) {
		var stores []struct {
			Name    string `json:"storage"`
			Active  int    `json:"active"`
			Enabled int    `json:"enabled"`
		}
		if err := p.storageQuery(ctx, host, known, "/nodes/"+node+"/storage", &stores); err != nil {
			return err
		}
		available := false
		for _, store := range stores {
			if store.Name == px.ISOStorage && store.Active == 1 && store.Enabled == 1 {
				available = true
			}
		}
		if !available {
			return fmt.Errorf("ISO storage %s is unavailable on %s", px.ISOStorage, node)
		}
	}
	return nil
}

func (p *Provisioner) storageQuery(ctx context.Context, host, known, path string, target any) error {
	result, err := hostssh.SSHRunArgvOutput(ctx, p.Exec, host, known, "pvesh", "get", path, "--output-format", "json")
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return executor.NewExitError(ctx, "pvesh get storage", result.ExitCode, result.Stderr)
	}
	if result.Truncated {
		return fmt.Errorf("proxmox storage observation truncated")
	}
	if err := json.Unmarshal([]byte(result.Stdout), target); err != nil {
		return fmt.Errorf("decode proxmox storage: %w", err)
	}
	return nil
}

func (p *Provisioner) isoStoragePath(ctx context.Context, cfg *config.Config, host, known string) (string, error) {
	if cfg.Provider.Proxmox.ISOStorage == "local" && len(placementNodes(cfg)) < 2 {
		return hostssh.DefaultProxmoxISODir, nil
	}
	volume := cfg.Provider.Proxmox.ISOStorage + ":iso/bootstrap.iso"
	result, err := hostssh.SSHRunArgv(ctx, p.Exec, host, known, "pvesm", "path", volume)
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return "", executor.NewExitError(ctx, "pvesm path", result.ExitCode, result.Stderr)
	}
	if result.Truncated {
		return "", fmt.Errorf("ISO storage path truncated")
	}
	path := strings.TrimSpace(result.Stdout)
	if filepath.Base(path) != "bootstrap.iso" {
		return "", fmt.Errorf("unexpected ISO storage path")
	}
	directory := filepath.Dir(path)
	if err := hostssh.ValidateISODir(directory); err != nil {
		return "", err
	}
	return directory, nil
}

func (p *Provisioner) verifySharedISOs(ctx context.Context, cfg *config.Config, host, known string, files []string) error {
	if len(placementNodes(cfg)) < 2 {
		return nil
	}
	storage := cfg.Provider.Proxmox.ISOStorage
	for _, node := range placementNodes(cfg) {
		var content []isoVolume
		if err := p.storageQuery(ctx, host, known, "/nodes/"+node+"/storage/"+storage+"/content", &content); err != nil {
			return err
		}
		for _, file := range files {
			volume := storage + ":iso/" + filepath.Base(file)
			if !slices.ContainsFunc(content, func(item isoVolume) bool { return item.Volume == volume }) {
				return fmt.Errorf("ISO %s is not visible on %s", volume, node)
			}
		}
	}
	return nil
}

type isoVolume struct {
	Volume string `json:"volid"`
}
