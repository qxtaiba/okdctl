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
	"github.com/qxtaiba/okdctl/internal/errtypes"
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

// GenerateInstallConfig renders install-config.yaml into outputDir using
// pull-secret and SSH key paths from cfg, then keeps a .backup copy before
// openshift-install consumes the original during manifest generation.
func (p *Phase) GenerateInstallConfig(_ context.Context, cfg *config.Config, outputDir string) error {
	if err := system.EnsureDir(outputDir); err != nil {
		return &errtypes.ConfigError{Msg: "failed to create output directory", Err: err}
	}

	pullSecret, err := os.ReadFile(cfg.Files.PullSecret)
	if err != nil {
		return &errtypes.AuthError{Msg: "failed to read pull secret", Err: err}
	}

	sshKey, err := os.ReadFile(cfg.Files.SSHPublicKey)
	if err != nil {
		return &errtypes.ConfigError{Msg: "failed to read SSH key", Err: err}
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
		return &errtypes.ConfigError{Msg: "failed to render install-config.yaml", Err: err}
	}

	// openshift-install consumes install-config.yaml during manifest generation
	backupPath := outputPath + ".backup"
	if err := system.CopyFileMode(outputPath, backupPath, 0o600); err != nil {
		return &errtypes.ConfigError{Msg: "failed to backup install-config.yaml", Err: err}
	}

	return nil
}

// GenerateManifests invokes "openshift-install create manifests" in clusterDir.
func (p *Phase) GenerateManifests(ctx context.Context, clusterDir string) error {
	_, err := p.Exec.RunChecked(ctx, "openshift-install", "create", "manifests", "--dir", clusterDir)
	if err != nil {
		return &errtypes.ClusterError{Msg: "openshift-install create manifests failed", Err: err}
	}

	return nil
}

// InjectCustomManifests copies user-supplied YAML from
// automation/config/manifests into clusterDir/openshift/, returning the
// count of files injected.
func (p *Phase) InjectCustomManifests(_ context.Context, projectRoot, clusterDir string) (int, error) {
	customDir := filepath.Join(projectRoot, "automation", "config", "manifests")

	if !system.DirExists(customDir) {
		return 0, nil // No custom manifests
	}

	entries, err := os.ReadDir(customDir)
	if err != nil {
		return 0, &errtypes.ConfigError{Msg: "failed to read custom manifests directory", Err: err}
	}

	openshiftDir := filepath.Join(clusterDir, "openshift")
	if err := system.EnsureDir(openshiftDir); err != nil {
		return 0, &errtypes.ConfigError{Msg: "failed to ensure openshift manifests directory", Err: err}
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
			return count, &errtypes.ConfigError{Msg: fmt.Sprintf("failed to inject %s", name), Err: err}
		}
		count++
	}

	return count, nil
}

// InjectCompactClusterManifests adds an ingress-controller placement
// manifest when the cluster has no workers (compact topology). With
// workers present, this is a no-op.
func (p *Phase) InjectCompactClusterManifests(_ context.Context, clusterDir string, workerCount, masterCount int) error {
	if workerCount > 0 {
		return nil
	}

	openshiftDir := filepath.Join(clusterDir, "openshift")
	if err := system.EnsureDir(openshiftDir); err != nil {
		return &errtypes.ConfigError{Msg: "failed to ensure openshift manifests directory", Err: err}
	}

	destPath := filepath.Join(openshiftDir, "99-ingress-controller-master-placement.yaml")
	if err := renderAndWrite(
		func() (string, error) {
			return templates.RenderCompactIngress(templates.CompactIngressData{Replicas: masterCount})
		},
		destPath, 0o644, "compact cluster ingress manifest",
	); err != nil {
		return &errtypes.ConfigError{Msg: "failed to render compact cluster ingress manifest", Err: err}
	}
	return nil
}

// GenerateIgnitionConfigs invokes "openshift-install create ignition-configs"
// and validates that each expected .ign file exists and is non-trivial in size.
func (p *Phase) GenerateIgnitionConfigs(ctx context.Context, clusterDir string) error {
	_, err := p.Exec.RunChecked(ctx, "openshift-install", "create", "ignition-configs", "--dir", clusterDir)
	if err != nil {
		return &errtypes.ClusterError{Msg: "openshift-install create ignition-configs failed", Err: err}
	}

	if err := p.ValidateIgnitionFiles(ctx, clusterDir); err != nil {
		return &errtypes.ConfigError{Msg: "ignition file validation failed", Err: err}
	}

	return nil
}

// ValidateIgnitionFiles verifies that bootstrap.ign, master.ign, and
// worker.ign exist in clusterDir and are at least 1 KiB.
func (p *Phase) ValidateIgnitionFiles(ctx context.Context, clusterDir string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	requiredFiles := []string{"bootstrap.ign", "master.ign", "worker.ign"}
	minSize := int64(1024) // ignition files are typically much larger

	for _, file := range requiredFiles {
		path := filepath.Join(clusterDir, file)

		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return &errtypes.ConfigError{Msg: fmt.Sprintf("%s was not generated", file)}
			}
			return &errtypes.ConfigError{Msg: fmt.Sprintf("failed to stat %s", file), Err: err}
		}

		if info.Size() < minSize {
			return &errtypes.ConfigError{Msg: fmt.Sprintf("%s is too small (%d bytes) - may be corrupted or empty", file, info.Size())}
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return &errtypes.ConfigError{Msg: fmt.Sprintf("failed to read %s", file), Err: err}
		}

		var js json.RawMessage
		if err := json.Unmarshal(content, &js); err != nil {
			return &errtypes.ConfigError{Msg: fmt.Sprintf("%s is not valid JSON", file), Err: err}
		}
	}

	return nil
}
