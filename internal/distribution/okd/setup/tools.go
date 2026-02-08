package setup

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/qxtaiba/okd-proxmox-cli/internal/addon"
	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/executor"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/download"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

// ExternalToolBinaries returns the list of external tool binaries installed to /usr/local/bin.
// Note: terraform is installed via dnf (HashiCorp repo) so it's not in this list.
func ExternalToolBinaries() []string {
	return []string{
		"yq",
		"helm",
		"sops",
	}
}

// externalTool represents an external tool that can be installed.
type externalTool string

const (
	toolTerraform externalTool = "terraform"
	toolYQ        externalTool = "yq"
	toolHelm      externalTool = "helm"
	toolSops      externalTool = "sops"
)

// coreTools returns tools that are always installed regardless of addon configuration.
func coreTools() []externalTool {
	return []externalTool{toolTerraform, toolYQ}
}

// addonRequiredTools returns tools required by enabled addons.
func addonRequiredTools(cfg *config.Config) []externalTool {
	seen := make(map[string]bool)
	var tools []externalTool

	for _, a := range addon.Enabled(cfg) {
		tp, ok := a.(addon.ToolProvider)
		if !ok {
			continue
		}
		for _, spec := range tp.RequiredTools() {
			if seen[spec.Name] {
				continue
			}
			seen[spec.Name] = true
			tools = append(tools, externalTool(spec.Name))
		}
	}
	return tools
}

// isToolInstalled checks if a tool is available in PATH.
func isToolInstalled(tool externalTool) bool {
	return executor.CommandExists(string(tool))
}

// InstallExternalTools installs core tools plus tools required by enabled addons.
func (p *Phase) InstallExternalTools(ctx context.Context, cfg *config.Config) error {
	for _, tool := range coreTools() {
		if err := p.installTool(ctx, tool); err != nil {
			return fmt.Errorf("failed to install %s: %w", tool, err)
		}
	}

	for _, tool := range addonRequiredTools(cfg) {
		if err := p.installTool(ctx, tool); err != nil {
			return fmt.Errorf("failed to install %s: %w", tool, err)
		}
	}
	return nil
}

// installTool installs a specific tool if not already present.
func (p *Phase) installTool(ctx context.Context, tool externalTool) error {
	if isToolInstalled(tool) {
		p.Log.Info(fmt.Sprintf("tools: %s already installed", tool))
		return nil
	}

	switch tool {
	case toolTerraform:
		return p.installTerraform(ctx)
	case toolYQ:
		return p.installYQ(ctx)
	case toolHelm:
		return p.installHelm(ctx)
	case toolSops:
		return p.installSops(ctx)
	default:
		p.Log.Warn(fmt.Sprintf("tools: no installer for %s, skipping (install manually)", tool))
		return nil
	}
}

// ════════════════════════════════════════════════════════════════════════════════
// TERRAFORM INSTALLATION
// ════════════════════════════════════════════════════════════════════════════════

// installTerraform installs terraform via HashiCorp RPM repository.
func (p *Phase) installTerraform(ctx context.Context) error {
	p.Log.Info("tools: installing terraform via hashicorp repository")

	repoURL := "https://rpm.releases.hashicorp.com/RHEL/hashicorp.repo"
	if err := runSudoCommand(ctx, "dnf", "config-manager", "--add-repo", repoURL); err != nil {
		return utils.WrapError("failed to add HashiCorp repository", err)
	}
	p.Log.Info("tools: hashicorp repository added")

	if err := runSudoCommand(ctx, "dnf", "install", "-y", "terraform"); err != nil {
		return utils.WrapError("failed to install terraform", err)
	}

	if !isToolInstalled(toolTerraform) {
		return fmt.Errorf("terraform installation verification failed")
	}

	version := getToolVersion("terraform", "--version")
	p.Log.Info(fmt.Sprintf("tools: terraform installed (%s)", version))
	return nil
}

// ════════════════════════════════════════════════════════════════════════════════
// YQ INSTALLATION
// ════════════════════════════════════════════════════════════════════════════════

// installYQ installs yq from GitHub releases.
// yq uses non-standard checksum format, so we skip verification (uses HTTPS).
func (p *Phase) installYQ(ctx context.Context) error {
	p.Log.Info("tools: installing yq from github releases")

	downloadURL := "https://github.com/mikefarah/yq/releases/latest/download/yq_linux_amd64"
	tempFile := filepath.Join(os.TempDir(), "yq_linux_amd64")

	if err := download.Download(ctx, download.Options{
		URL:         downloadURL,
		OutputPath:  tempFile,
		Description: "yq binary",
		Timeout:     2 * time.Minute,
	}); err != nil {
		return utils.WrapError("failed to download yq", err)
	}
	defer func() { _ = os.Remove(tempFile) }()

	if err := installBinaryToPath(ctx, tempFile, "yq"); err != nil {
		return err
	}

	if !isToolInstalled(toolYQ) {
		return fmt.Errorf("yq installation verification failed")
	}

	version := getToolVersion("yq", "--version")
	p.Log.Info(fmt.Sprintf("tools: yq installed (%s)", version))
	return nil
}

// ════════════════════════════════════════════════════════════════════════════════
// HELM INSTALLATION
// ════════════════════════════════════════════════════════════════════════════════

// installHelm installs helm from official releases.
func (p *Phase) installHelm(ctx context.Context) error {
	p.Log.Info("tools: installing helm from official releases")

	downloadURL := "https://get.helm.sh/helm-v3.17.3-linux-amd64.tar.gz"
	tempFile := filepath.Join(os.TempDir(), "helm-linux-amd64.tar.gz")

	if err := download.Download(ctx, download.Options{
		URL:         downloadURL,
		OutputPath:  tempFile,
		Description: "helm archive",
		Timeout:     2 * time.Minute,
	}); err != nil {
		return utils.WrapError("failed to download helm", err)
	}
	defer func() { _ = os.Remove(tempFile) }()

	extractDir := filepath.Join(os.TempDir(), "helm-extract")
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return utils.WrapError("failed to create extract directory", err)
	}
	defer func() { _ = os.RemoveAll(extractDir) }()

	if err := download.ExtractTarGz(ctx, download.ExtractOptions{
		ArchivePath:     tempFile,
		DestDir:         extractDir,
		StripComponents: 1, // Remove "linux-amd64/" prefix
		CleanupArchive:  true,
	}); err != nil {
		return utils.WrapError("failed to extract helm", err)
	}

	if err := installBinaryToPath(ctx, filepath.Join(extractDir, "helm"), "helm"); err != nil {
		return err
	}

	if !isToolInstalled(toolHelm) {
		return fmt.Errorf("helm installation verification failed")
	}

	version := getToolVersion("helm", "version")
	p.Log.Info(fmt.Sprintf("tools: helm installed (%s)", version))
	return nil
}

// ════════════════════════════════════════════════════════════════════════════════
// SOPS INSTALLATION
// ════════════════════════════════════════════════════════════════════════════════

// installSops installs sops from GitHub releases.
func (p *Phase) installSops(ctx context.Context) error {
	p.Log.Info("tools: installing sops from github releases")

	downloadURL := "https://github.com/getsops/sops/releases/download/v3.9.4/sops-v3.9.4.linux.amd64"
	tempFile := filepath.Join(os.TempDir(), "sops-linux-amd64")

	if err := download.Download(ctx, download.Options{
		URL:         downloadURL,
		OutputPath:  tempFile,
		Description: "sops binary",
		Timeout:     2 * time.Minute,
	}); err != nil {
		return utils.WrapError("failed to download sops", err)
	}
	defer func() { _ = os.Remove(tempFile) }()

	if err := installBinaryToPath(ctx, tempFile, "sops"); err != nil {
		return err
	}

	if !isToolInstalled(toolSops) {
		return fmt.Errorf("sops installation verification failed")
	}

	version := getToolVersion("sops", "--version")
	p.Log.Info(fmt.Sprintf("tools: sops installed (%s)", version))
	return nil
}

// ════════════════════════════════════════════════════════════════════════════════
// HELPER FUNCTIONS
// ════════════════════════════════════════════════════════════════════════════════

// installBinaryToPath copies a binary to /usr/local/bin with executable permissions.
func installBinaryToPath(ctx context.Context, srcPath, name string) error {
	destPath := filepath.Join("/usr/local/bin", name)

	if err := system.CopyFileWithElevation(ctx, srcPath, destPath, fmt.Sprintf("install %s", name)); err != nil {
		return utils.WrapError(fmt.Sprintf("failed to copy %s to /usr/local/bin", name), err)
	}

	if err := system.Chmod(ctx, destPath, "+x", fmt.Sprintf("make %s executable", name)); err != nil {
		return utils.WrapError(fmt.Sprintf("failed to set executable permissions on %s", name), err)
	}

	return nil
}

// runSudoCommand runs a command with sudo (or directly if already root).
func runSudoCommand(ctx context.Context, name string, args ...string) error {
	return system.RunSudo(ctx, name, args...)
}

// getToolVersion runs a tool with a version flag and returns the first line of output.
func getToolVersion(tool, flag string) string {
	cmd := exec.Command(tool, flag)
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) > 0 {
		return lines[0]
	}
	return "unknown"
}
