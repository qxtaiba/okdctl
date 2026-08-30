package setup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/provision"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/templates"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/system"
)

// zeroBytesFn wipes the pull-secret buffer; tests may replace it, production must not.
var zeroBytesFn = system.ZeroBytes

// readNoFollow reads path via lstat+O_NOFOLLOW, refusing a symlink at the final
// component (mirrors runlock.Acquire).
func readNoFollow(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("path %q is a symlink; refusing to follow", path)
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func renderAndWrite(render func() (string, error), path string, mode os.FileMode, errLabel string) error {
	content, err := render()
	if err != nil {
		return fmt.Errorf("render %s: %w", errLabel, err)
	}
	if err := system.AtomicWriteString(path, content, mode); err != nil {
		return fmt.Errorf("write %s: %w", errLabel, err)
	}
	return nil
}

// generateInstallConfig renders install-config.yaml from cfg's pull-secret and
// SSH key, backing it up before openshift-install consumes the original.
func (p *Phase) generateInstallConfig(ctx context.Context, cfg *config.Config, outputDir string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := system.EnsureDir(outputDir); err != nil {
		return &errtypes.ConfigError{Msg: "create output directory", Err: err}
	}

	pullSecret, err := readNoFollow(cfg.Files.PullSecret)
	if err != nil {
		return &errtypes.AuthError{Msg: "read pull secret", Err: err}
	}
	defer zeroBytesFn(pullSecret)
	if !json.Valid(pullSecret) {
		return &errtypes.AuthError{Msg: "pull secret is not valid JSON", Err: errtypes.ErrPullSecretInvalid}
	}

	sshKey, err := readNoFollow(cfg.Files.SSHPublicKey)
	if err != nil {
		return &errtypes.ConfigError{Msg: "read SSH key", Err: err}
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
		PullSecret:     string(bytes.TrimSpace(pullSecret)),
		SSHKey:         strings.TrimSpace(string(sshKey)),
		Architecture:   runtime.GOARCH,
	}

	outputPath := filepath.Join(outputDir, "install-config.yaml")
	if err := renderAndWrite(
		func() (string, error) { return templates.RenderInstallConfig(&data) },
		outputPath, 0o600, "install-config.yaml",
	); err != nil {
		return &errtypes.ConfigError{Msg: "generate install-config.yaml", Err: err}
	}

	// .backup doubles as the rollback artifact and AlreadyDone sentinel; a torn
	// write would poison both, so it must be atomic.
	rendered, err := os.ReadFile(outputPath)
	if err != nil {
		return &errtypes.ConfigError{Msg: "read install-config.yaml for backup", Err: err}
	}
	defer zeroBytesFn(rendered)
	backupPath := outputPath + ".backup"
	if err := system.AtomicWrite(backupPath, rendered, 0o600); err != nil {
		return &errtypes.ConfigError{Msg: "backup install-config.yaml", Err: err}
	}

	return nil
}

// ManifestsSentinel is the completion marker GenerateManifests writes;
// StepGenerateManifests' AlreadyDone also requires the manifests/ directory to
// exist.
func ManifestsSentinel(clusterDir string) string {
	return filepath.Join(clusterDir, "manifests", ".complete")
}

// IgnitionSentinel is the completion marker GenerateIgnitionConfigs writes;
// AlreadyDone keys on it rather than .ign presence since a partial write can
// leave malformed .ign files.
func IgnitionSentinel(clusterDir string) string {
	return filepath.Join(clusterDir, ".ignition.complete")
}

// manifestsGenerated reports whether create-manifests completed;
// IgnitionSentinel alone suffices since create ignition-configs also consumes
// manifests/ and ManifestsSentinel.
func manifestsGenerated(clusterDir string) bool {
	if system.FileExists(IgnitionSentinel(clusterDir)) {
		return true
	}
	return system.DirExists(filepath.Join(clusterDir, "manifests")) &&
		system.FileExists(ManifestsSentinel(clusterDir))
}

// restoreInstallConfigFromBackup restores install-config.yaml from .backup
// after a prior run's create-manifests consumed it; no-op otherwise.
func restoreInstallConfigFromBackup(clusterDir string) error {
	outputPath := filepath.Join(clusterDir, "install-config.yaml")
	backupPath := outputPath + ".backup"
	if system.FileExists(outputPath) || !system.FileExists(backupPath) {
		return nil
	}
	return system.CopyFileMode(backupPath, outputPath, 0o600)
}

// GenerateManifests expands install-config.yaml into the full manifest set
// under clusterDir via "openshift-install create manifests", then writes
// ManifestsSentinel.
func (p *Phase) GenerateManifests(ctx context.Context, clusterDir string) error {
	if err := restoreInstallConfigFromBackup(clusterDir); err != nil {
		return &errtypes.ConfigError{Msg: "restore install-config.yaml from backup", Err: err}
	}

	_, err := p.Exec.RunChecked(ctx, openshiftInstallBin, "create", "manifests", "--dir", clusterDir)
	if err != nil {
		return &errtypes.ClusterError{Msg: "openshift-install create manifests failed", Err: err}
	}

	if err := system.AtomicWrite(ManifestsSentinel(clusterDir), nil, 0o644); err != nil {
		return &errtypes.ClusterError{Msg: "write manifests sentinel", Err: err}
	}
	return nil
}

// InjectCustomManifests copies user YAML from automation/config/manifests
// into clusterDir/openshift/, returning the count injected.
func (p *Phase) InjectCustomManifests(ctx context.Context, projectRoot, clusterDir string) (int, error) {
	customDir := filepath.Join(projectRoot, "automation", "config", "manifests")

	if !system.DirExists(customDir) {
		return 0, nil
	}

	entries, err := os.ReadDir(customDir)
	if err != nil {
		return 0, &errtypes.ConfigError{Msg: "read custom manifests directory", Err: err}
	}

	openshiftDir := filepath.Join(clusterDir, openshiftSubdir)
	if err := system.EnsureDir(openshiftDir); err != nil {
		return 0, &errtypes.ConfigError{Msg: "ensure openshift manifests directory", Err: err}
	}

	count := 0
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return count, err
		}
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
			return count, &errtypes.ConfigError{Msg: fmt.Sprintf("inject %s", name), Err: err}
		}
		count++
	}

	return count, nil
}

// InjectCompactClusterManifests adds an ingress-controller placement
// manifest for compact (workerless) topologies; a no-op when workers exist.
func (p *Phase) InjectCompactClusterManifests(ctx context.Context, clusterDir string, workerCount, masterCount int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if workerCount > 0 {
		return nil
	}

	openshiftDir := filepath.Join(clusterDir, openshiftSubdir)
	if err := system.EnsureDir(openshiftDir); err != nil {
		return &errtypes.ConfigError{Msg: "ensure openshift manifests directory", Err: err}
	}

	destPath := filepath.Join(openshiftDir, "99-ingress-controller-master-placement.yaml")
	if err := renderAndWrite(
		func() (string, error) {
			return templates.RenderCompactIngress(templates.CompactIngressData{Replicas: masterCount})
		},
		destPath, 0o644, "compact cluster ingress manifest",
	); err != nil {
		return &errtypes.ConfigError{Msg: "inject compact cluster ingress manifest", Err: err}
	}
	return nil
}

// GenerateIgnitionConfigs runs "openshift-install create ignition-configs",
// validates the resulting .ign files, then writes IgnitionSentinel.
func (p *Phase) GenerateIgnitionConfigs(ctx context.Context, clusterDir string) error {
	_, err := p.Exec.RunChecked(ctx, openshiftInstallBin, "create", "ignition-configs", "--dir", clusterDir)
	if err != nil {
		return &errtypes.ClusterError{Msg: "openshift-install create ignition-configs failed", Err: err}
	}

	if err := p.ValidateIgnitionFiles(ctx, clusterDir); err != nil {
		return &errtypes.ConfigError{Msg: "ignition file validation failed", Err: err}
	}

	if err := system.AtomicWrite(IgnitionSentinel(clusterDir), nil, 0o644); err != nil {
		return &errtypes.ClusterError{Msg: "write ignition sentinel", Err: err}
	}
	return nil
}

// ValidateIgnitionFiles verifies bootstrap.ign, master.ign, and worker.ign
// exist in clusterDir and are at least 1 KiB.
func (p *Phase) ValidateIgnitionFiles(ctx context.Context, clusterDir string) error {
	minSize := int64(1024) // ignition files are typically much larger

	for _, file := range provision.IgnitionFilenames {
		if err := ctx.Err(); err != nil {
			return err
		}
		path := filepath.Join(clusterDir, file)

		info, err := os.Stat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return &errtypes.ConfigError{Msg: fmt.Sprintf("%s was not generated", file)}
			}
			return &errtypes.ConfigError{Msg: fmt.Sprintf("stat %s", file), Err: err}
		}

		if info.Size() < minSize {
			return &errtypes.ConfigError{Msg: fmt.Sprintf("%s is too small (%d bytes) - may be corrupted or empty", file, info.Size())}
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return &errtypes.ConfigError{Msg: fmt.Sprintf("read %s", file), Err: err}
		}

		var js json.RawMessage
		if err := json.Unmarshal(content, &js); err != nil {
			return &errtypes.ConfigError{Msg: fmt.Sprintf("%s is not valid JSON", file), Err: err}
		}
	}

	return nil
}
