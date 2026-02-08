package setup

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/executor"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

// collectISOFiles returns all .iso files from the given directory.
func collectISOFiles(isoDir string) ([]string, error) {
	entries, err := os.ReadDir(isoDir)
	if err != nil {
		return nil, err
	}

	var isoFiles []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".iso" {
			continue
		}
		isoFiles = append(isoFiles, filepath.Join(isoDir, entry.Name()))
	}
	return isoFiles, nil
}

// calculateTotalSize returns the total size of all files in bytes.
func calculateTotalSize(files []string) int64 {
	var totalSize int64
	for _, f := range files {
		if info, err := os.Stat(f); err == nil {
			totalSize += info.Size()
		}
	}
	return totalSize
}

// proxmoxHost extracts the hostname/IP from a host value that may include a port.
func proxmoxHost(host string) string {
	if strings.Contains(host, ":") {
		h, _, err := net.SplitHostPort(host)
		if err == nil {
			return h
		}
	}
	return host
}

// uploadISOsViaSCP uploads multiple ISO files to Proxmox via a single scp command.
func uploadISOsViaSCP(ctx context.Context, cmdRunner *executor.Executor, isoFiles []string, user, host, remotePath string) error {
	args := []string{"-o", "StrictHostKeyChecking=accept-new"}
	args = append(args, isoFiles...)
	args = append(args, fmt.Sprintf("%s@%s:%s/", user, host, remotePath))

	if err := cmdRunner.RunInteractive(ctx, "scp", args...); err != nil {
		return utils.WrapError("scp failed", err)
	}
	return nil
}

// UploadCustomISOsToProxmox uploads all custom ISOs to Proxmox storage.
// Uses a single scp command to upload all files at once (avoids multiple password prompts).
func (p *Phase) UploadCustomISOsToProxmox(ctx context.Context, cfg *config.Config, opts Options) error {
	if cfg.Provider.Proxmox == nil {
		return fmt.Errorf("proxmox provider configuration required")
	}

	isoDir := filepath.Join(opts.WorkDir, "custom-isos")
	if !system.DirExists(isoDir) {
		return fmt.Errorf("custom ISOs directory not found: %s", isoDir)
	}

	isoFiles, err := collectISOFiles(isoDir)
	if err != nil {
		return err
	}
	if len(isoFiles) == 0 {
		p.Log.Warn("iso: no iso files found to upload")
		return nil
	}

	host := proxmoxHost(cfg.Provider.Proxmox.Host)
	user := "root"
	remotePath := DefaultProxmoxISODir

	totalSizeMB := float64(calculateTotalSize(isoFiles)) / 1024 / 1024
	p.Log.Info(fmt.Sprintf("iso: uploading %d files (%.1f mb) to %s@%s:%s", len(isoFiles), totalSizeMB, user, host, remotePath))

	if err := uploadISOsViaSCP(ctx, p.Exec, isoFiles, user, host, remotePath); err != nil {
		return err
	}

	p.Log.Info(fmt.Sprintf("iso: uploaded %d files to proxmox storage", len(isoFiles)))
	return nil
}
