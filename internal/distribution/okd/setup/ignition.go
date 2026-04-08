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
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

func (p *Phase) GenerateInstallConfig(ctx context.Context, cfg *config.Config, outputDir string) error {
	if err := system.EnsureDir(outputDir); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	pullSecret, err := os.ReadFile(cfg.Files.PullSecret)
	if err != nil {
		return fmt.Errorf("failed to read pull secret: %w", err)
	}

	sshKey, err := os.ReadFile(cfg.Files.SSHPublicKey)
	if err != nil {
		return fmt.Errorf("failed to read SSH key: %w", err)
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
		return fmt.Errorf("failed to render install-config template: %w", err)
	}

	outputPath := filepath.Join(outputDir, "install-config.yaml")
	if err := system.AtomicWriteString(outputPath, content, 0600); err != nil {
		return fmt.Errorf("failed to write install-config.yaml: %w", err)
	}

	// openshift-install consumes install-config.yaml during manifest generation
	backupPath := outputPath + ".backup"
	if err := system.CopyFile(outputPath, backupPath); err != nil {
		return fmt.Errorf("failed to backup install-config.yaml: %w", err)
	}

	return nil
}

func (p *Phase) GenerateManifests(ctx context.Context, clusterDir string) error {
	result, err := p.Exec.Run(ctx, "openshift-install", "create", "manifests", "--dir", clusterDir)
	if err != nil {
		return fmt.Errorf("openshift-install create manifests failed: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("openshift-install create manifests failed: %s", result.Stderr)
	}

	return nil
}

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
			return count, fmt.Errorf("failed to inject %s: %w", name, err)
		}
		count++
	}

	return count, nil
}

func (p *Phase) InjectCompactClusterManifests(ctx context.Context, clusterDir string, workerCount, masterCount int) error {
	if workerCount > 0 {
		return nil
	}

	openshiftDir := filepath.Join(clusterDir, "openshift")
	if err := system.EnsureDir(openshiftDir); err != nil {
		return fmt.Errorf("failed to ensure openshift manifests directory: %w", err)
	}

	manifest, err := templates.RenderCompactIngress(templates.CompactIngressData{
		Replicas: masterCount,
	})
	if err != nil {
		return fmt.Errorf("failed to render compact ingress manifest: %w", err)
	}

	destPath := filepath.Join(openshiftDir, "99-ingress-controller-master-placement.yaml")
	if err := system.AtomicWriteString(destPath, manifest, 0644); err != nil {
		return fmt.Errorf("failed to write compact cluster ingress manifest: %w", err)
	}

	return nil
}

func (p *Phase) GenerateIgnitionConfigs(ctx context.Context, clusterDir string) error {
	result, err := p.Exec.Run(ctx, "openshift-install", "create", "ignition-configs", "--dir", clusterDir)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("openshift-install create ignition-configs failed: %s", result.Stderr)
	}

	if err := p.ValidateIgnitionFiles(clusterDir); err != nil {
		return fmt.Errorf("ignition file validation failed: %w", err)
	}

	return nil
}

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
			return fmt.Errorf("failed to stat %s: %w", file, err)
		}

		if info.Size() < minSize {
			return fmt.Errorf("%s is too small (%d bytes) - may be corrupted or empty", file, info.Size())
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", file, err)
		}

		var js json.RawMessage
		if err := json.Unmarshal(content, &js); err != nil {
			return fmt.Errorf("%s is not valid JSON: %w", file, err)
		}
	}

	return nil
}
