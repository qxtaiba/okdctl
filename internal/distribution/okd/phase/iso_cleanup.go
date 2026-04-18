package phase

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/qxtaiba/okdctl/internal/executor"
)

// RemoteISOParams carries the connection parameters needed to clean ISOs from
// a Proxmox host over SSH. Host must be the bare hostname or IP (no port).
type RemoteISOParams struct {
	Host string
	Node string
	Exec *executor.Executor
	Log  *slog.Logger
}

// refuseUnsafeISOPath rejects any path that is not exactly
// <isoDir>/fedora-coreos-*.iso. The guard prevents a config typo from
// pointing an SSH rm at an arbitrary path on the Proxmox host.
func refuseUnsafeISOPath(isoDir, path string) error {
	cleaned := filepath.Clean(path)
	dir := filepath.Dir(cleaned)
	base := filepath.Base(cleaned)

	if dir != filepath.Clean(isoDir) {
		return fmt.Errorf("refusing unsafe remote path %q: not inside %s", path, isoDir)
	}
	if !strings.HasPrefix(base, "fedora-coreos-") || !strings.HasSuffix(base, ".iso") {
		return fmt.Errorf("refusing unsafe remote path %q: not a fedora-coreos-*.iso filename", path)
	}
	return nil
}

// vmReferencesISO returns true when any VM on the node has the given ISO
// path in its CD-ROM/boot configuration, determined by running pvesh over SSH.
func vmReferencesISO(ctx context.Context, p *RemoteISOParams, isoPath string) (bool, error) {
	result, err := p.Exec.Run(ctx, "ssh",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "BatchMode=yes",
		"root@"+p.Host,
		fmt.Sprintf("pvesh get /nodes/%s/qemu --output-format json 2>/dev/null || echo '[]'", p.Node),
	)
	if err != nil {
		return false, fmt.Errorf("ssh pvesh failed: %w", err)
	}
	// A simple substring search is sufficient: Proxmox embeds the ISO filename
	// verbatim in the cdrom/boot config. Full JSON parsing adds a large
	// dependency for no meaningful gain here.
	isoBase := filepath.Base(isoPath)
	return strings.Contains(result.Stdout, isoBase), nil
}

// RemoveFCOSISOFromProxmox removes fedora-coreos-*.iso files from isoDir on
// the Proxmox host over SSH. Files still referenced by a running VM are
// skipped with a warning. The path safety check runs before every rm.
func RemoveFCOSISOFromProxmox(ctx context.Context, p *RemoteISOParams, isoDir string) error {
	result, err := p.Exec.Run(ctx, "ssh",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "BatchMode=yes",
		"root@"+p.Host,
		fmt.Sprintf("ls %s/fedora-coreos-*.iso 2>/dev/null || true", isoDir),
	)
	if err != nil {
		return fmt.Errorf("ssh ls failed: %w", err)
	}

	files := parseSSHFileList(result.Stdout)
	if len(files) == 0 {
		p.Log.Info("iso: no fedora-coreos-*.iso found on proxmox host, nothing to remove")
		return nil
	}

	for _, f := range files {
		if err := refuseUnsafeISOPath(isoDir, f); err != nil {
			p.Log.Warn(fmt.Sprintf("iso: skipping %s: %v", f, err))
			continue
		}

		inUse, err := vmReferencesISO(ctx, p, f)
		if err != nil {
			p.Log.Warn(fmt.Sprintf("iso: could not check vm references for %s: %v — skipping", filepath.Base(f), err))
			continue
		}
		if inUse {
			p.Log.Warn(fmt.Sprintf("iso: %s is still referenced by a running vm — skipping removal", filepath.Base(f)))
			continue
		}

		if _, rmErr := p.Exec.Run(ctx, "ssh",
			"-o", "StrictHostKeyChecking=accept-new",
			"-o", "BatchMode=yes",
			"root@"+p.Host,
			"rm -f "+f,
		); rmErr != nil {
			p.Log.Warn(fmt.Sprintf("iso: failed to remove %s: %v", filepath.Base(f), rmErr))
			continue
		}
		p.Log.Info(fmt.Sprintf("iso: removed %s from proxmox host", filepath.Base(f)))
	}

	return nil
}

func parseSSHFileList(output string) []string {
	var files []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files
}
