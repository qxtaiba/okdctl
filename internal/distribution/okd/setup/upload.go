package setup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/system"
)

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

func calculateTotalSize(files []string) int64 {
	var totalSize int64
	for _, f := range files {
		if info, err := os.Stat(f); err == nil {
			totalSize += info.Size()
		}
	}
	return totalSize
}

func uploadISOsViaSCP(ctx context.Context, cmdRunner *executor.Executor, isoFiles []string, user, host, remotePath string) error {
	args := []string{"-o", "StrictHostKeyChecking=accept-new"}
	args = append(args, isoFiles...)
	args = append(args, fmt.Sprintf("%s@%s:%s/", user, host, remotePath))

	if err := cmdRunner.RunInteractive(ctx, "scp", args...); err != nil {
		return fmt.Errorf("scp failed: %w", err)
	}
	return nil
}

// UploadCustomISOsToProxmox uploads all custom ISOs to Proxmox storage via a
// single scp command (avoids multiple password prompts).
func (p *Phase) UploadCustomISOsToProxmox(ctx context.Context, cfg *config.Config, opts *Options) error {
	if cfg.Provider.Proxmox == nil {
		return &errtypes.ConfigError{Msg: "proxmox provider configuration required"}
	}

	isoDir := filepath.Join(opts.WorkDir, "custom-isos")
	if !system.DirExists(isoDir) {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("custom ISOs directory not found: %s", isoDir)}
	}

	isoFiles, err := collectISOFiles(isoDir)
	if err != nil {
		return &errtypes.ConfigError{Msg: "failed to collect ISO files", Err: err}
	}
	if len(isoFiles) == 0 {
		p.Log.Warn("iso: no iso files found to upload")
		return nil
	}

	host := phase.ProxmoxBareHost(cfg.Provider.Proxmox.Host)
	user := "root"
	remotePath := phase.DefaultProxmoxISODir

	totalSizeMB := float64(calculateTotalSize(isoFiles)) / 1024 / 1024
	p.Log.Info("iso: uploading", "count", len(isoFiles), "size_mb", fmt.Sprintf("%.1f", totalSizeMB), "user", user, "host", host, "path", remotePath)

	if err := uploadISOsViaSCP(ctx, p.Exec, isoFiles, user, host, remotePath); err != nil {
		return &errtypes.NetworkError{Msg: "scp upload to proxmox failed", Err: err}
	}

	p.Log.Info(fmt.Sprintf("iso: uploaded %d files to proxmox storage", len(isoFiles)))
	return nil
}
