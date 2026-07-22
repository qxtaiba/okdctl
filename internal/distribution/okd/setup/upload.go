package setup

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/download"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/infrastructure/proxmox/hostssh"
	"github.com/qxtaiba/okdctl/internal/sshpin"
	"github.com/qxtaiba/okdctl/internal/system"
)

// remoteISO256 runs sha256sum on remotePath/filename over SSH and returns the
// hex digest. Any SSH or parse failure returns ("", err).
func remoteISO256(ctx context.Context, exec *executor.Executor, host, knownHostsPath, remotePath, filename string) (string, error) {
	if err := hostssh.ValidateISODir(remotePath); err != nil {
		return "", fmt.Errorf("remoteISO256: %w", err)
	}
	if err := hostssh.ValidateRemoteFilename(filename); err != nil {
		return "", fmt.Errorf("remoteISO256: %w", err)
	}
	target := remotePath + "/" + filename
	result, err := hostssh.SSHRunArgv(ctx, exec, host, knownHostsPath, "sha256sum", "--", target)
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
func isoUploadNeeded(ctx context.Context, exec *executor.Executor, host, knownHostsPath, remotePath, localPath string) bool {
	localHash, err := download.CalculateChecksum(ctx, localPath)
	if err != nil {
		return true
	}
	remoteHash, err := remoteISO256(ctx, exec, host, knownHostsPath, remotePath, filepath.Base(localPath))
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

// proxmoxSCPUser is fixed: ISO uploads target /var/lib/vz, writable only
// by root on a stock Proxmox VE install.
const proxmoxSCPUser = "root"

// uploadISOsViaSCP scps ISOs to the Proxmox host one file at a time.
// Per-file invocations mean a SIGINT or network drop mid-batch leaves
// already-uploaded files intact; the next run resumes only the corrupt tail.
// When knownHostsPath is non-empty the scp call enforces strict host-key
// checking against that file, matching sshBaseArgs policy in hostssh/ssh.go.
// An empty path falls back to accept-new TOFU, preserving behaviour for
// operators without a configured fingerprint.
func uploadISOsViaSCP(ctx context.Context, cmdRunner *executor.Executor, isoFiles []string, host, remotePath, knownHostsPath string) error {
	var baseArgs []string
	if knownHostsPath != "" {
		baseArgs = []string{
			"-o", "UserKnownHostsFile=" + knownHostsPath,
			"-o", "StrictHostKeyChecking=yes",
			"-o", "BatchMode=yes",
		}
	} else {
		baseArgs = []string{
			"-o", "StrictHostKeyChecking=accept-new",
			"-o", "BatchMode=yes",
		}
	}
	dest := fmt.Sprintf("%s@%s:%s/", proxmoxSCPUser, host, remotePath)
	for _, f := range isoFiles {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !isoUploadNeeded(ctx, cmdRunner, host, knownHostsPath, remotePath, f) {
			continue
		}
		args := append(slices.Clone(baseArgs), f, dest)
		if err := cmdRunner.RunInteractive(ctx, "scp", args...); err != nil {
			return fmt.Errorf("scp %s failed: %w", filepath.Base(f), err)
		}
	}
	return nil
}

// UploadCustomISOsToProxmox uploads all custom ISOs to Proxmox storage via a
// single scp command (avoids multiple password prompts).
func (p *Phase) UploadCustomISOsToProxmox(ctx context.Context, cfg *config.Config, opts *Options) error {
	if cfg.Provider.Proxmox == nil {
		return &errtypes.ConfigError{Msg: msgProxmoxProviderRequired}
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

	host := hostssh.ProxmoxBareHost(cfg.Provider.Proxmox.Host)
	remotePath := hostssh.DefaultProxmoxISODir

	knownHostsPath, err := sshpin.Verify(ctx, host, cfg.Provider.Proxmox.SSHHostFingerprint, cfg.Provider.Proxmox.RequirePinnedFingerprint, p.Log)
	if err != nil {
		return &errtypes.NetworkError{Msg: "proxmox host key verification failed", Err: err}
	}

	var toUpload []string
	for _, f := range isoFiles {
		if isoUploadNeeded(ctx, p.Exec, host, knownHostsPath, remotePath, f) {
			toUpload = append(toUpload, f)
		} else {
			p.Log.Info("iso: skipping unchanged", "file", filepath.Base(f))
		}
	}

	if len(toUpload) == 0 {
		p.Log.Info("iso: all isos already up to date on proxmox storage")
		return nil
	}

	totalSizeMB := float64(calculateTotalSize(toUpload)) / 1024 / 1024
	roundedMB := math.Round(totalSizeMB*10) / 10
	p.Log.Info("iso: uploading", "count", len(toUpload), "size_mb", roundedMB, "user", proxmoxSCPUser, "host", host, "path", remotePath)

	if err := uploadISOsViaSCP(ctx, p.Exec, toUpload, host, remotePath, knownHostsPath); err != nil {
		return &errtypes.NetworkError{Msg: "scp upload to proxmox failed", Err: err}
	}

	p.Log.Info("iso: uploaded files to proxmox storage", "count", len(toUpload))
	return nil
}

// isoUploadAlreadyDone returns true when every local ISO has an identical
// sha256 on the Proxmox host. Any SSH failure or absent Proxmox config
// conservatively returns (false, nil) — the conservative-not-done choice
// lets Exec surface the real failure rather than silently skipping the
// upload.
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
	host := hostssh.ProxmoxBareHost(cfg.Provider.Proxmox.Host)
	remotePath := hostssh.DefaultProxmoxISODir
	knownHostsPath, err := sshpin.Verify(ctx, host, cfg.Provider.Proxmox.SSHHostFingerprint, cfg.Provider.Proxmox.RequirePinnedFingerprint, p.Log)
	if err != nil {
		return false, err
	}
	if slices.ContainsFunc(isoFiles, func(f string) bool {
		return isoUploadNeeded(ctx, p.Exec, host, knownHostsPath, remotePath, f)
	}) {
		return false, nil
	}
	return true, nil
}
