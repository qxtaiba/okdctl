package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/templates"
	"github.com/qxtaiba/okdctl/internal/system"
)

// renderAndWrite calls render and atomically writes the result to path,
// wrapping errors with errLabel.
func renderAndWrite(render func() (string, error), path string, mode os.FileMode, errLabel string) error {
	content, err := render()
	if err != nil {
		return fmt.Errorf("failed to render %s: %w", errLabel, err)
	}
	if err := system.AtomicWriteString(path, content, mode); err != nil {
		return fmt.Errorf("failed to write %s: %w", errLabel, err)
	}
	return nil
}

func (p *Phase) GenerateInstallConfig(_ context.Context, cfg *config.Config, outputDir string) error {
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
		Architecture:   runtime.GOARCH,
	}

	outputPath := filepath.Join(outputDir, "install-config.yaml")
	if err := renderAndWrite(
		func() (string, error) { return templates.RenderInstallConfig(&data) },
		outputPath, 0o600, "install-config.yaml",
	); err != nil {
		return err
	}

	// openshift-install consumes install-config.yaml during manifest generation
	backupPath := outputPath + ".backup"
	if err := system.CopyFile(outputPath, backupPath); err != nil {
		return fmt.Errorf("failed to backup install-config.yaml: %w", err)
	}

	return nil
}

func (p *Phase) GenerateManifests(ctx context.Context, clusterDir string) error {
	_, err := p.Exec.RunChecked(ctx, "openshift-install", "create", "manifests", "--dir", clusterDir)
	if err != nil {
		return fmt.Errorf("openshift-install create manifests failed: %w", err)
	}

	return nil
}

func (p *Phase) InjectCustomManifests(_ context.Context, projectRoot, clusterDir string) (int, error) {
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

func (p *Phase) InjectCompactClusterManifests(_ context.Context, clusterDir string, workerCount, masterCount int) error {
	if workerCount > 0 {
		return nil
	}

	openshiftDir := filepath.Join(clusterDir, "openshift")
	if err := system.EnsureDir(openshiftDir); err != nil {
		return fmt.Errorf("failed to ensure openshift manifests directory: %w", err)
	}

	destPath := filepath.Join(openshiftDir, "99-ingress-controller-master-placement.yaml")
	return renderAndWrite(
		func() (string, error) {
			return templates.RenderCompactIngress(templates.CompactIngressData{Replicas: masterCount})
		},
		destPath, 0o644, "compact cluster ingress manifest",
	)
}

func (p *Phase) GenerateIgnitionConfigs(ctx context.Context, clusterDir string) error {
	_, err := p.Exec.RunChecked(ctx, "openshift-install", "create", "ignition-configs", "--dir", clusterDir)
	if err != nil {
		return fmt.Errorf("openshift-install create ignition-configs failed: %w", err)
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
