package setup

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/qxtaiba/okd-proxmox-cli/internal/addon"
	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/executor"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/download"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

func downloadArch() string {
	switch runtime.GOARCH {
	case "arm64":
		return "arm64"
	default:
		return "amd64"
	}
}

// ExternalToolBinaries returns the list of external tool binaries installed to /usr/local/bin.
// Note: terraform is installed via dnf (HashiCorp repo) so it's not in this list.
func ExternalToolBinaries() []string {
	return []string{
		"yq",
		"helm",
		"sops",
	}
}

type externalTool string

const (
	toolTerraform externalTool = "terraform"
	toolYQ        externalTool = "yq"
	toolHelm      externalTool = "helm"
	toolSops      externalTool = "sops"
)

func coreTools() []externalTool {
	return []externalTool{toolTerraform, toolYQ}
}

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

func isToolInstalled(tool externalTool) bool {
	return executor.CommandExists(string(tool))
}

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

func (p *Phase) installTerraform(ctx context.Context) error {
	p.Log.Info("tools: installing terraform via hashicorp repository")

	switch p.OS.Family {
	case "debian":
		if err := system.RunSudo(ctx, "sh", "-c",
			"wget -O- https://apt.releases.hashicorp.com/gpg | gpg --dearmor -o /usr/share/keyrings/hashicorp-archive-keyring.gpg"); err != nil {
			return fmt.Errorf("failed to add HashiCorp GPG key: %w", err)
		}
		if err := system.RunSudo(ctx, "sh", "-c",
			`echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main" > /etc/apt/sources.list.d/hashicorp.list`); err != nil {
			return fmt.Errorf("failed to add HashiCorp repository: %w", err)
		}
		if err := system.RunSudo(ctx, "apt-get", "update"); err != nil {
			return fmt.Errorf("failed to update package list: %w", err)
		}
	default: // rhel family
		repoURL := "https://rpm.releases.hashicorp.com/RHEL/hashicorp.repo"
		if err := runSudoCommand(ctx, "dnf", "config-manager", "--add-repo", repoURL); err != nil {
			return fmt.Errorf("failed to add HashiCorp repository: %w", err)
		}
	}

	p.Log.Info("tools: hashicorp repository added")

	if err := p.Pkg.Install(ctx, []string{"terraform"}, p.Log); err != nil {
		return fmt.Errorf("failed to install terraform: %w", err)
	}

	if !isToolInstalled(toolTerraform) {
		return fmt.Errorf("terraform installation verification failed")
	}

	version := getToolVersion("terraform", "--version")
	p.Log.Info(fmt.Sprintf("tools: terraform installed (%s)", version))
	return nil
}

type binaryInstallSpec struct {
	name            string
	url             string
	versionFlag     string
	archiveBinary   string // if non-empty, download is a tar.gz; value is the binary path within the archive
	stripComponents int
}

func (p *Phase) installBinary(ctx context.Context, spec binaryInstallSpec) error {
	p.Log.Info(fmt.Sprintf("tools: installing %s", spec.name))

	tempFile := filepath.Join(os.TempDir(), spec.name+"-download")
	if err := download.Download(ctx, download.Options{
		URL: spec.url, OutputPath: tempFile,
		Description: spec.name, Timeout: 2 * time.Minute, Logger: p.Log,
	}); err != nil {
		return fmt.Errorf("failed to download %s: %w", spec.name, err)
	}
	defer func() { _ = os.Remove(tempFile) }()

	srcPath := tempFile
	if spec.archiveBinary != "" {
		extractDir := filepath.Join(os.TempDir(), spec.name+"-extract")
		if err := os.MkdirAll(extractDir, 0755); err != nil {
			return fmt.Errorf("failed to create extract directory: %w", err)
		}
		defer func() { _ = os.RemoveAll(extractDir) }()
		if err := download.ExtractTarGz(ctx, download.ExtractOptions{
			ArchivePath: tempFile, DestDir: extractDir,
			StripComponents: spec.stripComponents, CleanupArchive: true, Logger: p.Log,
		}); err != nil {
			return fmt.Errorf("failed to extract %s: %w", spec.name, err)
		}
		srcPath = filepath.Join(extractDir, spec.archiveBinary)
	}

	if err := installBinaryToPath(ctx, srcPath, spec.name); err != nil {
		return err
	}
	if !isToolInstalled(externalTool(spec.name)) {
		return fmt.Errorf("%s installation verification failed", spec.name)
	}
	p.Log.Info(fmt.Sprintf("tools: %s installed (%s)", spec.name, getToolVersion(spec.name, spec.versionFlag)))
	return nil
}

func (p *Phase) installYQ(ctx context.Context) error {
	arch := downloadArch()
	return p.installBinary(ctx, binaryInstallSpec{
		name: "yq", versionFlag: "--version",
		url: fmt.Sprintf("https://github.com/mikefarah/yq/releases/latest/download/yq_linux_%s", arch),
	})
}

func (p *Phase) installHelm(ctx context.Context) error {
	arch := downloadArch()
	return p.installBinary(ctx, binaryInstallSpec{
		name: "helm", versionFlag: "version",
		url: fmt.Sprintf("https://get.helm.sh/helm-v3.17.3-linux-%s.tar.gz", arch),
		archiveBinary: "helm", stripComponents: 1,
	})
}

func (p *Phase) installSops(ctx context.Context) error {
	arch := downloadArch()
	return p.installBinary(ctx, binaryInstallSpec{
		name: "sops", versionFlag: "--version",
		url: fmt.Sprintf("https://github.com/getsops/sops/releases/download/v3.9.4/sops-v3.9.4.linux.%s", arch),
	})
}

func installBinaryToPath(ctx context.Context, srcPath, name string) error {
	destPath := filepath.Join("/usr/local/bin", name)

	if err := system.CopyFileWithElevation(ctx, srcPath, destPath, fmt.Sprintf("install %s", name)); err != nil {
		return fmt.Errorf("failed to copy %s to /usr/local/bin: %w", name, err)
	}

	if err := system.Chmod(ctx, destPath, "+x", fmt.Sprintf("make %s executable", name)); err != nil {
		return fmt.Errorf("failed to set executable permissions on %s: %w", name, err)
	}

	return nil
}

func runSudoCommand(ctx context.Context, name string, args ...string) error {
	return system.RunSudo(ctx, name, args...)
}

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
