package setup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/download"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/fetchplan"
	"github.com/qxtaiba/okdctl/internal/system"
)

// DownloadOKDTools dispatches to the OCI release-image path (default) or the
// legacy GitHub-tarball path when OKDCTL_RELEASE_SOURCE=github is set.
func (p *Phase) DownloadOKDTools(ctx context.Context, version string, opts *Options) error {
	src := ResolveReleaseSource(opts.ReleaseSource)
	if src == ReleaseSourceGitHub {
		p.Log.Warn("tools: OKDCTL_RELEASE_SOURCE=github is deprecated; the GitHub tarball path will be removed in a future release")
		return p.downloadOKDToolsFromGitHub(ctx, version, opts)
	}
	return p.DownloadOKDToolsViaImage(ctx, version, opts, fetchplan.DefaultResolver{})
}

// downloadOKDToolsFromGitHub is the legacy GitHub-tarball path retained as a
// deprecation fallback. Call only via DownloadOKDTools.
func (p *Phase) downloadOKDToolsFromGitHub(ctx context.Context, version string, opts *Options) error {
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
