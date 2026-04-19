package setup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/download"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/system"
)

const defaultOKDReleaseBaseURL = "https://github.com/okd-project/okd/releases/download"

// ResolveReleaseBaseURL returns the OKD release base URL for downloads.
// OKDCTL_OKD_RELEASE_URL (env) overrides cfg.Deployment.OKDReleaseBaseURL,
// which overrides the upstream GitHub default — the env wins so air-gapped
// operators can redirect a single invocation without editing the config.
func ResolveReleaseBaseURL(cfg *config.Config) string {
	if v := os.Getenv("OKDCTL_OKD_RELEASE_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	if cfg.Deployment.OKDReleaseBaseURL != "" {
		return strings.TrimRight(cfg.Deployment.OKDReleaseBaseURL, "/")
	}
	return defaultOKDReleaseBaseURL
}

var toolURLEnvVars = map[string]string{
	string(toolHelm): "OKDCTL_HELM_URL",
	string(toolSops): "OKDCTL_SOPS_URL",
	string(toolYQ):   "OKDCTL_YQ_URL",
}

var toolVersionEnvVars = map[string]string{
	string(toolHelm): "OKDCTL_HELM_VERSION",
	string(toolSops): "OKDCTL_SOPS_VERSION",
	string(toolYQ):   "OKDCTL_YQ_VERSION",
}

// ResolveToolURL returns the download URL template for the named binary tool.
// Resolution order: env var > cfg.Deployment.ToolVersions[tool].URLTemplate >
// defaultURL. The returned string may contain {version} and {arch} placeholders
// that callers substitute via strings.NewReplacer; a URL with no placeholders
// passes through verbatim.
func ResolveToolURL(tool, defaultURL string, cfg *config.Config) string {
	if envKey, ok := toolURLEnvVars[tool]; ok {
		if v := os.Getenv(envKey); v != "" {
			return v
		}
	}
	if cfg != nil {
		if ov, ok := cfg.Deployment.ToolVersions[tool]; ok && ov.URLTemplate != "" {
			return ov.URLTemplate
		}
	}
	return defaultURL
}

// ResolveToolVersion returns the version string used to expand the {version}
// placeholder in a tool URL template. Resolution order: env var >
// cfg.Deployment.ToolVersions[tool].Version > defaultVersion.
func ResolveToolVersion(tool, defaultVersion string, cfg *config.Config) string {
	if envKey, ok := toolVersionEnvVars[tool]; ok {
		if v := os.Getenv(envKey); v != "" {
			return v
		}
	}
	if cfg != nil {
		if ov, ok := cfg.Deployment.ToolVersions[tool]; ok && ov.Version != "" {
			return ov.Version
		}
	}
	return defaultVersion
}

// DownloadOKDTools fetches openshift-install and oc for the given OKD
// version, verifies checksums when available, extracts the binaries, and
// then delegates to InstallToolsToSystem for final placement.
func (p *Phase) DownloadOKDTools(ctx context.Context, version string, opts *Options) error {
	if err := system.EnsureDir(opts.DownloadDir); err != nil {
		return &errtypes.ConfigError{Msg: "failed to create download directory", Err: err}
	}

	baseURL := fmt.Sprintf("%s/%s", opts.OKDReleaseBaseURL, version)
	checksumsURL := fmt.Sprintf("%s/sha256sum.txt", baseURL)

	tools := []struct {
		name     string
		filename string
		binary   string
	}{
		{
			name:     "openshift-install",
			filename: fmt.Sprintf("openshift-install-linux-%s.tar.gz", version),
			binary:   "openshift-install",
		},
		{
			name:     "oc",
			filename: fmt.Sprintf("openshift-client-linux-%s.tar.gz", version),
			binary:   "oc",
		},
	}

	for _, tool := range tools {
		archivePath := filepath.Join(opts.DownloadDir, tool.filename)
		toolURL := fmt.Sprintf("%s/%s", baseURL, tool.filename)

		binaryPath := filepath.Join(opts.DownloadDir, tool.binary)
		if system.FileExists(binaryPath) && !opts.SkipDownloads {
			p.Log.Info(fmt.Sprintf("tools: using existing %s binary", tool.name))
			continue
		}

		checksum, err := download.FetchChecksum(ctx, checksumsURL, tool.filename)
		checksumSkipped := false
		if err != nil {
			p.Log.Warn(fmt.Sprintf("tools: proceeding without checksum validation for %s", tool.name))
			checksum = ""
			checksumSkipped = true
		}

		downloadOpts := &download.Options{
			URL:              toolURL,
			OutputPath:       archivePath,
			ExpectedChecksum: checksum,
			Description:      tool.name,
			Logger:           p.Log,
		}
		if err := download.Download(ctx, downloadOpts); err != nil {
			return &errtypes.NetworkError{Msg: fmt.Sprintf("failed to download %s", tool.name), Err: err}
		}

		extractOpts := download.ExtractOptions{
			ArchivePath:    archivePath,
			DestDir:        opts.DownloadDir,
			CleanupArchive: true,
			Logger:         p.Log,
		}
		if err := download.ExtractTarGz(ctx, extractOpts); err != nil {
			return &errtypes.NetworkError{Msg: fmt.Sprintf("failed to extract %s", tool.name), Err: err}
		}

		// Defense-in-depth: when checksum validation was skipped, verify the
		// extracted binary at least exists and is non-empty so a corrupt or
		// missing artifact fails loudly instead of silently installing.
		if checksumSkipped {
			fi, statErr := os.Stat(binaryPath)
			if statErr != nil {
				return &errtypes.NetworkError{Msg: fmt.Sprintf("tools: extracted %s binary missing at %s", tool.name, binaryPath), Err: statErr}
			}
			if fi.Size() == 0 {
				return &errtypes.NetworkError{Msg: fmt.Sprintf("tools: extracted %s binary at %s is empty", tool.name, binaryPath)}
			}
		}
	}

	return p.InstallToolsToSystem(ctx, opts.DownloadDir)
}

// InstallToolsToSystem copies the downloaded OKD binaries from srcDir into
// p.BinDir (empty falls back to phase.DefaultBinDir) with executable mode.
func (p *Phase) InstallToolsToSystem(_ context.Context, srcDir string) error {
	binaries := []string{"openshift-install", "oc", "kubectl"}
	destDir := phase.BinDirOrDefault(p.BinDir)

	for _, binary := range binaries {
		srcPath := filepath.Join(srcDir, binary)
		if !system.FileExists(srcPath) {
			continue
		}

		destPath := filepath.Join(destDir, binary)

		if err := system.CopyFile(srcPath, destPath); err != nil {
			return &errtypes.ConfigError{Msg: fmt.Sprintf("failed to install %s", binary), Err: err}
		}

		if err := system.MakeExecutable(destPath); err != nil {
			p.Log.Warn("tools: failed to set executable permission", "binary", binary, "err", err)
		}

		if !system.FileExists(destPath) {
			return &errtypes.ConfigError{Msg: fmt.Sprintf("tools: %s not found at %s after install", binary, destPath)}
		}

		p.Log.Info("tools: installed binary", "binary", binary, "path", destPath)
	}

	return nil
}
