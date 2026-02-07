package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/templates"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

// GenerateInstallConfig creates the install-config.yaml for openshift-install.
func (p *Phase) GenerateInstallConfig(ctx context.Context, cfg *config.Config, outputDir string) error {
	if err := system.EnsureDir(outputDir); err != nil {
		return utils.WrapError("failed to create output directory", err)
	}

	pullSecret, err := os.ReadFile(cfg.Files.PullSecret)
	if err != nil {
		return utils.WrapError("failed to read pull secret", err)
	}

	sshKey, err := os.ReadFile(cfg.Files.SSHPublicKey)
	if err != nil {
		return utils.WrapError("failed to read SSH key", err)
	}

	hostPrefix := cfg.Networking.HostPrefix
	if hostPrefix == 0 {
		hostPrefix = 23
	}

	data := templates.InstallConfigData{
		ClusterName:    cfg.Cluster.Name,
		BaseDomain:     cfg.Cluster.Domain,
		MasterReplicas: cfg.Topology.ControlPlane.Count,
		WorkerReplicas: cfg.Topology.Workers.Count,
		ClusterCIDR:    cfg.Networking.PodCIDR,
		HostPrefix:     hostPrefix,
		MachineCIDR:    cfg.Networking.MachineCIDR,
		ServiceCIDR:    cfg.Networking.ServiceCIDR,
		PullSecret:     strings.TrimSpace(string(pullSecret)),
		SSHKey:         strings.TrimSpace(string(sshKey)),
	}

	content, err := templates.RenderInstallConfig(data)
	if err != nil {
		return utils.WrapError("failed to render install-config template", err)
	}

	outputPath := filepath.Join(outputDir, "install-config.yaml")
	if err := system.AtomicWriteString(outputPath, content, 0600); err != nil {
		return utils.WrapError("failed to write install-config.yaml", err)
	}

	// openshift-install consumes install-config.yaml, so back it up
	backupPath := outputPath + ".backup"
	if err := system.CopyFile(outputPath, backupPath); err != nil {
		return utils.WrapError("failed to backup install-config.yaml", err)
	}

	return nil
}

// GenerateManifests runs openshift-install create manifests.
func (p *Phase) GenerateManifests(ctx context.Context, clusterDir string) error {
	result, err := p.Exec.Run(ctx, "openshift-install", "create", "manifests", "--dir", clusterDir)
	if err != nil {
		return utils.WrapError("openshift-install create manifests failed", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("openshift-install create manifests failed: %s", result.Stderr)
	}

	schedulerConfig := filepath.Join(clusterDir, "manifests", "cluster-scheduler-02-config.yml")
	if system.FileExists(schedulerConfig) {
		content, err := os.ReadFile(schedulerConfig)
		if err != nil {
			return utils.WrapError("failed to read scheduler config", err)
		}

		newContent := strings.Replace(string(content), "mastersSchedulable: true", "mastersSchedulable: false", 1)
		if newContent == string(content) {
			p.Log.Warn("manifests: mastersSchedulable setting not found in scheduler config")
		}
		if err := system.AtomicWriteString(schedulerConfig, newContent, 0644); err != nil {
			return utils.WrapError("failed to write scheduler config", err)
		}
	}

	return nil
}

// InjectCustomManifests copies custom manifests into the cluster config.
func (p *Phase) InjectCustomManifests(ctx context.Context, projectRoot, clusterDir string) (int, error) {
	customDir := filepath.Join(projectRoot, "automation", "config", "manifests")

	if !system.DirExists(customDir) {
		return 0, nil // No custom manifests
	}

	entries, err := os.ReadDir(customDir)
	if err != nil {
		return 0, err
	}

	openshiftDir := filepath.Join(clusterDir, "openshift")
	if err := system.EnsureDir(openshiftDir); err != nil {
		return 0, err
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}

		srcPath := filepath.Join(customDir, name)
		destPath := filepath.Join(openshiftDir, name)

		if err := system.CopyFile(srcPath, destPath); err != nil {
			return count, utils.WrapErrorf(err, "failed to inject %s", name)
		}
		count++
	}

	return count, nil
}

// GenerateIgnitionConfigs runs openshift-install create ignition-configs.
func (p *Phase) GenerateIgnitionConfigs(ctx context.Context, clusterDir string) error {
	result, err := p.Exec.Run(ctx, "openshift-install", "create", "ignition-configs", "--dir", clusterDir)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("openshift-install create ignition-configs failed: %s", result.Stderr)
	}

	if err := p.ValidateIgnitionFiles(clusterDir); err != nil {
		return utils.WrapError("ignition file validation failed", err)
	}

	return nil
}

// ValidateIgnitionFiles checks that ignition files exist and have valid content.
func (p *Phase) ValidateIgnitionFiles(clusterDir string) error {
	requiredFiles := []string{"bootstrap.ign", "master.ign", "worker.ign"}
	minSize := int64(1024) // ignition files are typically much larger

	for _, file := range requiredFiles {
		path := filepath.Join(clusterDir, file)

		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("%s was not generated", file)
			}
			return utils.WrapErrorf(err, "failed to stat %s", file)
		}

		if info.Size() < minSize {
			return fmt.Errorf("%s is too small (%d bytes) - may be corrupted or empty", file, info.Size())
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return utils.WrapErrorf(err, "failed to read %s", file)
		}

		var js json.RawMessage
		if err := json.Unmarshal(content, &js); err != nil {
			return utils.WrapErrorf(err, "%s is not valid JSON", file)
		}
	}

	return nil
}
