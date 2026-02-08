package setup

import (
	"context"
	"fmt"
	"path/filepath"

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
		if err != nil {
			p.Log.Warn(fmt.Sprintf("tools: proceeding without checksum validation for %s", tool.name))
			checksum = ""
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
	}

	return p.InstallToolsToSystem(ctx, opts.DownloadDir)
}

func (p *Phase) InstallToolsToSystem(ctx context.Context, srcDir string) error {
	binaries := []string{"openshift-install", "oc", "kubectl"}
	destDir := "/usr/local/bin"

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

		p.Log.Info(fmt.Sprintf("tools: installed %s to %s", binary, destPath))
	}

	return nil
}
