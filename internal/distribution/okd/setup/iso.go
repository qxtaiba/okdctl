package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/paths"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/templates"
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

// writePreInstallScript writes the rendered pre-install script to a temp file.
func writePreInstallScript(script string) (string, error) {
	f, err := os.CreateTemp("", "pre-install-*.sh")
	if err != nil {
		return "", fmt.Errorf("failed to create pre-install script: %w", err)
	}

	if _, err := f.WriteString(script); err != nil {
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

// writeInstallerTriggerIgnition creates a temp Ignition config that writes a
// minimal file to /etc/coreos/installer.d/ so the directory is non-empty when
// systemd evaluates ConditionDirectoryNotEmpty on coreos-installer.service.
// If sshKey is non-empty it is added to the core user so the live environment
// is reachable via SSH (useful for debugging installer failures).
func writeInstallerTriggerIgnition(sshKey string) (string, error) {
	ign := map[string]any{
		"ignition": map[string]any{"version": "3.3.0"},
		"storage": map[string]any{
			"files": []map[string]any{{
				"path": "/etc/coreos/installer.d/00-install-trigger.yaml",
				"mode": 420,
				"contents": map[string]any{
					"source": "data:,fetch-retries%3A%200%0A",
				},
			}},
		},
	}
	if sshKey != "" {
		ign["passwd"] = map[string]any{
			"users": []map[string]any{{
				"name":              "core",
				"sshAuthorizedKeys": []string{sshKey},
			}},
		}
	}

	data, err := json.Marshal(ign)
	if err != nil {
		return "", fmt.Errorf("failed to marshal installer trigger ignition: %w", err)
	}

	f, err := os.CreateTemp("", "installer-trigger-*.ign")
	if err != nil {
		return "", fmt.Errorf("failed to create installer trigger ignition: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("failed to write installer trigger ignition: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("failed to close installer trigger ignition: %w", err)
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

	kargsParams := LiveKargsParams{
		NodeIP:      node.IP,
		Gateway:     gateway,
		Netmask:     netmask,
		DNS:         dns,
		Interface:   iface,
		IgnitionURL: ignitionURL,
	}

	args := []string{"iso", "customize"}
	for _, karg := range BuildLiveKargs(kargsParams) {
		args = append(args, "--live-karg-append", karg)
	}
	for _, karg := range BuildDestKargs(kargsParams) {
		args = append(args, "--dest-karg-append", karg)
	}

	// All nodes use a pre-install script that discovers the OS disk by serial
	// via lsblk. Workers also get the data disk wiped; for bootstrap/masters
	// the data serial is absent so the wipe is safely skipped.
	script, err := templates.RenderPreInstall(templates.PreInstallData{
		OSSerial:   "OS-DISK",
		DataSerial: "CEPH-DATA",
	})
	if err != nil {
		return err
	}
	scriptPath, err := writePreInstallScript(script)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(scriptPath) }()

	// coreos-installer.service requires either coreos.inst.install_dev on the
	// kernel command line or a non-empty /etc/coreos/installer.d/ directory.
	// The pre-install script populates installer.d at runtime, but the
	// directory is still empty when systemd evaluates the condition — so the
	// service is skipped and the script never runs (chicken-and-egg).
	//
	// We solve this with a live ignition fragment that seeds installer.d with
	// a harmless config file. Ignition runs during initramfs (before systemd
	// evaluates service conditions) so the directory is non-empty in time.
	// Unlike a coreos.inst.install_dev karg, this does NOT override the
	// dest-device written by the pre-install script's serial-based discovery
	// (kargs are evaluated after installer.d and would take precedence).
	var sshKey string
	if cfg.Files.SSHPublicKey != "" {
		keyPath := system.ExpandPath(cfg.Files.SSHPublicKey)
		if b, err := os.ReadFile(keyPath); err == nil {
			sshKey = strings.TrimSpace(string(b))
		}
	}
	triggerPath, err := writeInstallerTriggerIgnition(sshKey)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(triggerPath) }()
	args = append(args, "--live-ignition", triggerPath)
	args = append(args, "--pre-install", scriptPath)

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
