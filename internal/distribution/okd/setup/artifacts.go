package setup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/phase"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/download"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

func (p *Phase) DownloadOKDTools(ctx context.Context, version string, opts Options) error {
	if err := system.EnsureDir(opts.DownloadDir); err != nil {
		return fmt.Errorf("failed to create download directory: %w", err)
	}

	baseURL := fmt.Sprintf("https://github.com/okd-project/okd/releases/download/%s", version)
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

		downloadOpts := download.Options{
			URL:              toolURL,
			OutputPath:       archivePath,
			ExpectedChecksum: checksum,
			Description:      tool.name,
			Logger:           p.Log,
		}
		if err := download.Download(ctx, downloadOpts); err != nil {
			return fmt.Errorf("failed to download %s: %w", tool.name, err)
		}

		extractOpts := download.ExtractOptions{
			ArchivePath:    archivePath,
			DestDir:        opts.DownloadDir,
			CleanupArchive: true,
			Logger:         p.Log,
		}
		if err := download.ExtractTarGz(ctx, extractOpts); err != nil {
			return fmt.Errorf("failed to extract %s: %w", tool.name, err)
		}

		// Defense-in-depth: when checksum validation was skipped, verify the
		// extracted binary at least exists and is non-empty so a corrupt or
		// missing artifact fails loudly instead of silently installing.
		if checksumSkipped {
			fi, statErr := os.Stat(binaryPath)
			if statErr != nil {
				return fmt.Errorf("tools: extracted %s binary missing at %s: %w", tool.name, binaryPath, statErr)
			}
			if fi.Size() == 0 {
				return fmt.Errorf("tools: extracted %s binary at %s is empty", tool.name, binaryPath)
			}
		}
	}

	return p.InstallToolsToSystem(ctx, opts.DownloadDir)
}

func (p *Phase) InstallToolsToSystem(ctx context.Context, srcDir string) error {
	binaries := []string{"openshift-install", "oc", "kubectl"}
	destDir := phase.DefaultBinDir

	for _, binary := range binaries {
		srcPath := filepath.Join(srcDir, binary)
		if !system.FileExists(srcPath) {
			continue
		}

		destPath := filepath.Join(destDir, binary)

		if err := system.CopyFileWithElevation(ctx, srcPath, destPath, fmt.Sprintf("install %s", binary)); err != nil {
			return fmt.Errorf("failed to install %s: %w", binary, err)
		}

		if err := system.Chmod(ctx, destPath, "+x", fmt.Sprintf("set %s executable", binary)); err != nil {
			p.Log.Warn(fmt.Sprintf("tools: failed to set executable permission for %s: %v", binary, err))
		}

		if !system.FileExists(destPath) {
			return fmt.Errorf("tools: %s not found at %s after install", binary, destPath)
		}

		p.Log.Info(fmt.Sprintf("tools: installed %s to %s", binary, destPath))
	}

	return nil
}
