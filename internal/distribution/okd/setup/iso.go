package setup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/paths"
	"github.com/qxtaiba/okd-proxmox-cli/internal/executor"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

// BuildCustomISOs creates customized CoreOS ISOs for all cluster nodes.
func (p *Phase) BuildCustomISOs(ctx context.Context, cfg *config.Config, opts Options) error {
	isoDir := filepath.Join(opts.WorkDir, "custom-isos")
	if err := system.EnsureDir(isoDir); err != nil {
		return err
	}

	clusterDir := paths.ClusterConfigDir(opts.WorkDir)

	if !executor.CommandExists("coreos-installer") {
		return fmt.Errorf("coreos-installer not found - please install it first")
	}

	fcosISO, err := p.findOrDownloadFCOSISO(ctx, cfg, opts)
	if err != nil {
		return utils.WrapError("failed to find or download FCOS ISO", err)
	}

	nodes, err := p.BuildNodeList(cfg)
	if err != nil {
		return utils.WrapError("failed to build node list", err)
	}

	for _, node := range nodes {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		p.LogInfo(fmt.Sprintf("iso: building custom coreos iso for %s", node.Name))

		if err := p.buildNodeISO(ctx, cfg, node, clusterDir, fcosISO, isoDir); err != nil {
			return utils.WrapErrorf(err, "failed to build ISO for %s", node.Name)
		}
	}

	return nil
}

// writePreInstallScript writes the worker pre-install script to a temp file.
func writePreInstallScript() (string, error) {
	f, err := os.CreateTemp("", "pre-install-*.sh")
	if err != nil {
		return "", fmt.Errorf("failed to create pre-install script: %w", err)
	}

	if _, err := f.WriteString(WorkerPreInstallScript()); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("failed to write pre-install script: %w", err)
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("failed to close pre-install script: %w", err)
	}

	if err := os.Chmod(f.Name(), 0755); err != nil {
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("failed to chmod pre-install script: %w", err)
	}

	return f.Name(), nil
}

// buildNodeISO creates a customized CoreOS ISO for a specific node using iso customize.
func (p *Phase) buildNodeISO(ctx context.Context, cfg *config.Config, node NodeInfo, clusterDir, fcosISO, outputDir string) error {
	isoName := fmt.Sprintf("%s.iso", node.Name)
	outputPath := filepath.Join(outputDir, isoName)

	if system.FileExists(outputPath) {
		if err := os.Remove(outputPath); err != nil {
			return utils.WrapErrorf(err, "failed to remove existing ISO %s", outputPath)
		}
	}

	gateway, netmask, dns, iface := ExtractNetworkConfig(cfg)
	ignitionURL := BuildIgnitionURLForNode(cfg, node.Role)

	kargs := BuildLiveKargs(LiveKargsParams{
		NodeIP:      node.IP,
		Gateway:     gateway,
		Netmask:     netmask,
		DNS:         dns,
		Interface:   iface,
		IgnitionURL: ignitionURL,
	})

	args := []string{"iso", "customize"}
	for _, karg := range kargs {
		args = append(args, "--live-karg-append", karg)
	}

	switch node.Role {
	case "worker":
		// workers have two disks — use a pre-install script that discovers the
		// OS disk by serial via lsblk and wipes the data disk.
		scriptPath, err := writePreInstallScript()
		if err != nil {
			return err
		}
		defer func() { _ = os.Remove(scriptPath) }()
		args = append(args, "--pre-install", scriptPath)
	default:
		// bootstrap and master VMs have a single disk; /dev/sda is always correct.
		args = append(args, "--live-karg-append", "coreos.inst.install_dev=/dev/sda")
	}

	args = append(args, "-o", outputPath, fcosISO)

	result, err := p.Exec.Run(ctx, "coreos-installer", args...)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("coreos-installer failed: %s", result.Stderr)
	}

	return nil
}
