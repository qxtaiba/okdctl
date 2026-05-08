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
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/system"
)

// remoteISO256 runs sha256sum on remotePath/filename over SSH and returns the
// hex digest. Any SSH or parse failure returns ("", err).
func remoteISO256(ctx context.Context, exec *executor.Executor, host, remotePath, filename string) (string, error) {
	target := remotePath + "/" + filename
	result, err := phase.SSHRunArgv(ctx, exec, host, "sha256sum", "--", target)
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("sha256sum %s exited %d", target, result.ExitCode)
	}
	fields := strings.Fields(result.Stdout)
	if len(fields) == 0 {
		return "", fmt.Errorf("sha256sum %s: empty output", target)
	}
	return fields[0], nil
}

// isoUploadNeeded returns false when the remote file's sha256 matches the
// local file. Any error (SSH transport, parse, local hash failure) returns
// true so the caller falls back to uploading.
func isoUploadNeeded(ctx context.Context, exec *executor.Executor, host, remotePath, localPath string) bool {
	localHash, err := download.CalculateChecksum(localPath)
	if err != nil {
		return true
	}
	remoteHash, err := remoteISO256(ctx, exec, host, remotePath, filepath.Base(localPath))
	if err != nil {
		return true
	}
	return localHash != remoteHash
}

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

// uploadISOsViaSCP scps ISOs to the Proxmox host. The first call uses
// StrictHostKeyChecking=accept-new (TOFU); a planned proxmox.host_fingerprint
// config field will pre-seed known_hosts and close that window. See
// README §security-considerations.
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

	var toUpload []string
	for _, f := range isoFiles {
		if isoUploadNeeded(ctx, p.Exec, host, remotePath, f) {
			toUpload = append(toUpload, f)
		} else {
			p.Log.Info(fmt.Sprintf("iso: skipping unchanged %s", filepath.Base(f)))
		}
	}

	if len(toUpload) == 0 {
		p.Log.Info("iso: all isos already up to date on proxmox storage")
		return nil
	}

	totalSizeMB := float64(calculateTotalSize(toUpload)) / 1024 / 1024
	p.Log.Info("iso: uploading", "count", len(toUpload), "size_mb", fmt.Sprintf("%.1f", totalSizeMB), "user", user, "host", host, "path", remotePath)

	if err := uploadISOsViaSCP(ctx, p.Exec, toUpload, user, host, remotePath); err != nil {
		return &errtypes.NetworkError{Msg: "scp upload to proxmox failed", Err: err}
	}

	p.Log.Info(fmt.Sprintf("iso: uploaded %d files to proxmox storage", len(toUpload)))
	return nil
}

// isoUploadAlreadyDone returns true when every local ISO has an identical
// sha256 on the Proxmox host. Any SSH failure or absent Proxmox config
// conservatively returns (false, nil).
func (p *Phase) isoUploadAlreadyDone(ctx context.Context, cfg *config.Config, opts *Options) (bool, error) {
	if cfg.Provider.Proxmox == nil {
		return false, nil
	}
	isoDir := filepath.Join(opts.WorkDir, "custom-isos")
	if !system.DirExists(isoDir) {
		return false, nil
	}
	isoFiles, err := collectISOFiles(isoDir)
	// Conservative: any error or empty list means "not done" so Exec runs and
	// surfaces the real failure mode.
	if err != nil || len(isoFiles) == 0 {
		return false, nil //nolint:nilerr // intentional: caller treats false as "Exec must run"
	}
	host := phase.ProxmoxBareHost(cfg.Provider.Proxmox.Host)
	remotePath := phase.DefaultProxmoxISODir
	for _, f := range isoFiles {
		if isoUploadNeeded(ctx, p.Exec, host, remotePath, f) {
			return false, nil
		}
	}
	return true, nil
}
