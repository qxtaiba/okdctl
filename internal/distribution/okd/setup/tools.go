package setup

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/addon"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/download"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/httputil"
	"github.com/qxtaiba/okdctl/internal/platform"
	"github.com/qxtaiba/okdctl/internal/system"
)

type externalTool string

const (
	toolTerraform externalTool = "terraform"
	toolYQ        externalTool = "yq"
	toolHelm      externalTool = "helm"
	toolSops      externalTool = "sops"
)

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

// InstallExternalTools installs terraform, yq, and any addon-declared tools.
func (p *Phase) InstallExternalTools(ctx context.Context, cfg *config.Config) error {
	tools := append([]externalTool{toolTerraform, toolYQ}, addonRequiredTools(cfg)...)
	for _, tool := range tools {
		if err := p.installTool(ctx, tool, cfg); err != nil {
			return fmt.Errorf("failed to install %s: %w", tool, err)
		}
	}
	return nil
}

// binaryTools defines the install specs for tools fetched as prebuilt
// binaries. url may contain {version} and {arch} placeholders substituted
// at install time; defaultVersion seeds {version} when no override is set.
// yq intentionally uses GitHub's /releases/latest/download/ redirect rather
// than a {version} placeholder — it is the only path that resolves to "the
// current release" without a concrete tag.
var binaryTools = map[externalTool]binaryInstallSpec{
	toolYQ: {
		name: "yq", versionFlag: "--version",
		url: "https://github.com/mikefarah/yq/releases/latest/download/yq_linux_{arch}",
	},
	toolHelm: {
		name: "helm", versionFlag: "version", defaultVersion: "v3.17.3",
		url:           "https://get.helm.sh/helm-{version}-linux-{arch}.tar.gz",
		archiveBinary: "helm", stripComponents: 1,
	},
	toolSops: {
		name: "sops", versionFlag: "--version", defaultVersion: "v3.9.4",
		url: "https://github.com/getsops/sops/releases/download/{version}/sops-{version}.linux.{arch}",
	},
}

func (p *Phase) installTool(ctx context.Context, tool externalTool, cfg *config.Config) error {
	if isToolInstalled(tool) {
		p.Log.Info(fmt.Sprintf("tools: %s already installed", tool))
		return nil
	}

	if tool == toolTerraform {
		return p.installTerraform(ctx)
	}

	spec, ok := binaryTools[tool]
	if !ok {
		p.Log.Warn(fmt.Sprintf("tools: no installer for %s, skipping (install manually)", tool))
		return nil
	}
	resolvedURL := ResolveToolURL(string(tool), spec.url, cfg)
	resolvedVersion := ResolveToolVersion(string(tool), spec.defaultVersion, cfg)
	if resolvedURL != spec.url || resolvedVersion != spec.defaultVersion {
		p.Log.Info("tools: using override", "tool", tool, "version", resolvedVersion, "url", resolvedURL)
	}
	spec.url = strings.NewReplacer(
		"{version}", resolvedVersion,
		"{arch}", platform.DownloadArch(),
	).Replace(resolvedURL)
	return p.installBinary(ctx, &spec)
}

func (p *Phase) installTerraform(ctx context.Context) error {
	p.Log.Info("tools: installing terraform via hashicorp repository")

	switch p.OS.Family {
	case platform.FamilyDebian:
		if err := installHashiCorpDebianRepo(ctx); err != nil {
			return err
		}
	default: // rhel family
		repoURL := "https://rpm.releases.hashicorp.com/RHEL/hashicorp.repo"
		if err := exec.CommandContext(ctx, "dnf", "config-manager", "--add-repo", repoURL).Run(); err != nil {
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

	version := getToolVersion(ctx, "terraform", "--version")
	p.Log.Info(fmt.Sprintf("tools: terraform installed (%s)", version))
	return nil
}

type binaryInstallSpec struct {
	name            string
	url             string
	defaultVersion  string
	versionFlag     string
	archiveBinary   string // if non-empty, download is a tar.gz; value is the binary path within the archive
	stripComponents int
}

func (p *Phase) installBinary(ctx context.Context, spec *binaryInstallSpec) error {
	p.Log.Info(fmt.Sprintf("tools: installing %s", spec.name))

	tempFile := filepath.Join(os.TempDir(), spec.name+"-download")
	if err := download.Download(ctx, &download.Options{
		URL: spec.url, OutputPath: tempFile,
		Description: spec.name, Timeout: 2 * time.Minute, Logger: p.Log,
	}); err != nil {
		return fmt.Errorf("failed to download %s: %w", spec.name, err)
	}
	defer func() { _ = os.Remove(tempFile) }()

	srcPath := tempFile
	if spec.archiveBinary != "" {
		extractDir := filepath.Join(os.TempDir(), spec.name+"-extract")
		if err := os.MkdirAll(extractDir, 0o755); err != nil {
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
	p.Log.Info("tools: installed", "tool", spec.name, "version", getToolVersion(ctx, spec.name, spec.versionFlag))
	return nil
}

func installBinaryToPath(_ context.Context, srcPath, name string) error {
	destPath := filepath.Join(phase.DefaultBinDir, name)

	if err := system.CopyFile(srcPath, destPath); err != nil {
		return fmt.Errorf("failed to copy %s to %s: %w", name, phase.DefaultBinDir, err)
	}

	if err := system.MakeExecutable(destPath); err != nil {
		return fmt.Errorf("failed to set executable permissions on %s: %w", name, err)
	}

	return nil
}

func getToolVersion(ctx context.Context, tool, flag string) string {
	cmd := exec.CommandContext(ctx, tool, flag)
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

func installHashiCorpDebianRepo(ctx context.Context) error {
	gpgPath := "/usr/share/keyrings/hashicorp-archive-keyring.gpg"

	gpgTmp, err := system.WriteTempFile("hashicorp-gpg", 0o600, func(f *os.File) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://apt.releases.hashicorp.com/gpg", http.NoBody)
		if err != nil {
			return err
		}
		resp, err := httputil.New(httputil.TimeoutShort).Do(req)
		if err != nil {
			return fmt.Errorf("fetch hashicorp gpg key: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("fetch hashicorp gpg key: status %d", resp.StatusCode)
		}
		if _, err := io.Copy(f, resp.Body); err != nil {
			return fmt.Errorf("copy hashicorp gpg key: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to download HashiCorp GPG key: %w", err)
	}
	defer func() { _ = os.Remove(gpgTmp) }()

	if err := exec.CommandContext(ctx, "gpg", "--dearmor", "-o", gpgPath, gpgTmp).Run(); err != nil {
		return fmt.Errorf("failed to dearmor HashiCorp GPG key: %w", err)
	}

	codeCmd := exec.CommandContext(ctx, "lsb_release", "-cs")
	codeOut, err := codeCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to detect debian codename: %w", err)
	}
	codename := strings.TrimSpace(string(codeOut))

	listContent := fmt.Sprintf("deb [signed-by=%s] https://apt.releases.hashicorp.com %s main\n", gpgPath, codename)
	listPath := "/etc/apt/sources.list.d/hashicorp.list"
	listTmp, err := system.WriteTempFile("hashicorp-list", 0o600, func(f *os.File) error {
		_, err := f.WriteString(listContent)
		return err
	})
	if err != nil {
		return fmt.Errorf("failed to write HashiCorp repo list: %w", err)
	}
	defer func() { _ = os.Remove(listTmp) }()

	if err := system.CopyFile(listTmp, listPath); err != nil {
		return fmt.Errorf("failed to install HashiCorp repo list: %w", err)
	}
	return exec.CommandContext(ctx, "apt-get", "update").Run()
}
