package phase

import (
	"context"
	"encoding/json"
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

// shellSingleQuote wraps s in single quotes and escapes any embedded single
// quotes using the POSIX end-quote/literal-quote/reopen idiom, making the
// result safe to pass to a remote shell command string.
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// validateISODir rejects isoDir values that contain shell metacharacters or
// are not absolute paths. The check prevents command injection when isoDir is
// interpolated into the find command string.
func validateISODir(isoDir string) error {
	if !filepath.IsAbs(isoDir) {
		return fmt.Errorf("isoDir must be an absolute path, got %q", isoDir)
	}
	// Reject characters that have meaning to /bin/sh.
	const dangerous = ";|&$`\\\"!(){}<>~*?[]#"
	for _, r := range dangerous {
		if strings.ContainsRune(isoDir, r) {
			return fmt.Errorf("isoDir %q contains unsafe character %q", isoDir, string(r))
		}
	}
	return nil
}

// proxmoxQEMUList is the minimal shape returned by pvesh for the QEMU list.
type proxmoxQEMUList []map[string]json.RawMessage

// vmReferencesISO returns true when any VM on the node has the given ISO
// filename in its device-mapping configuration, determined by running pvesh
// over SSH.
//
// Proxmox device fields (ide0-3, sata0-5, scsi0-30, virtio0-15) hold
// comma-separated key=value pairs such as "local:iso/fedora-coreos-40.iso,media=cdrom".
// Checking only those fields avoids false positives from description or notes.
func vmReferencesISO(ctx context.Context, p *RemoteISOParams, isoBase string) (bool, error) {
	result, err := SSHRun(ctx, p.Exec, p.Host,
		fmt.Sprintf("pvesh get /nodes/%s/qemu --output-format json 2>/dev/null || echo '[]'", p.Node),
	)
	if err != nil {
		return false, fmt.Errorf("ssh pvesh failed: %w", err)
	}

	var vms proxmoxQEMUList
	if err := json.Unmarshal([]byte(result.Stdout), &vms); err != nil {
		return false, fmt.Errorf("pvesh output not valid json: %w", err)
	}

	for _, vm := range vms {
		if vmDevicesReferenceISO(vm, isoBase) {
			return true, nil
		}
	}
	return false, nil
}

// deviceFields enumerates Proxmox VM disk/CD-ROM config keys.
var deviceFields = func() []string {
	fields := []string{"boot", "bootdisk"}
	for i := 0; i <= 3; i++ {
		fields = append(fields, fmt.Sprintf("ide%d", i))
	}
	for i := 0; i <= 5; i++ {
		fields = append(fields, fmt.Sprintf("sata%d", i))
	}
	for i := 0; i <= 30; i++ {
		fields = append(fields, fmt.Sprintf("scsi%d", i))
	}
	for i := 0; i <= 15; i++ {
		fields = append(fields, fmt.Sprintf("virtio%d", i))
	}
	return fields
}()

// vmDevicesReferenceISO returns true if any device-mapping field in vm
// contains a comma-separated segment whose value ends with the given isoBase.
func vmDevicesReferenceISO(vm map[string]json.RawMessage, isoBase string) bool {
	for _, field := range deviceFields {
		raw, ok := vm[field]
		if !ok {
			continue
		}
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			continue
		}
		// Segments are comma-separated; device entries may be bare paths or
		// "file=<storage>:<pool>/<name>" key=value form.
		for _, seg := range strings.Split(value, ",") {
			seg = strings.TrimSpace(seg)
			if v, found := strings.CutPrefix(seg, "file="); found {
				seg = v
			}
			if strings.HasSuffix(seg, isoBase) {
				return true
			}
		}
	}
	return false
}

// RemoveFCOSISOFromProxmox removes fedora-coreos-*.iso files from isoDir on
// the Proxmox host over SSH. Files still referenced by a running VM are
// skipped with a warning. The path safety check runs before every rm.
func RemoveFCOSISOFromProxmox(ctx context.Context, p *RemoteISOParams, isoDir string) error {
	if err := validateISODir(isoDir); err != nil {
		return err
	}

	// find -print0 avoids newline-splitting ambiguity that plagued the old ls approach.
	findCmd := fmt.Sprintf(
		"find %s -maxdepth 1 -name 'fedora-coreos-*.iso' -type f -print0 2>/dev/null || true",
		isoDir,
	)
	result, err := SSHRun(ctx, p.Exec, p.Host, findCmd)
	if err != nil {
		return fmt.Errorf("ssh find failed: %w", err)
	}

	files := parseNullDelimitedFileList(result.Stdout)
	if len(files) == 0 {
		p.Log.Info("iso: no fedora-coreos-*.iso found on proxmox host, nothing to remove")
		return nil
	}

	for _, f := range files {
		if err := refuseUnsafeISOPath(isoDir, f); err != nil {
			p.Log.Warn(fmt.Sprintf("iso: skipping %s: %v", f, err))
			continue
		}

		isoBase := filepath.Base(f)
		inUse, err := vmReferencesISO(ctx, p, isoBase)
		if err != nil {
			p.Log.Warn(fmt.Sprintf("iso: could not check vm references for %s: %v — skipping", isoBase, err))
			continue
		}
		if inUse {
			p.Log.Warn(fmt.Sprintf("iso: %s is still referenced by a running vm — skipping removal", isoBase))
			continue
		}

		// Shell-single-quote the path so filenames with spaces or metacharacters
		// reach rm as a single literal argument.
		if _, rmErr := SSHRun(ctx, p.Exec, p.Host, "rm -f "+shellSingleQuote(f)); rmErr != nil {
			p.Log.Warn(fmt.Sprintf("iso: failed to remove %s: %v", isoBase, rmErr))
			continue
		}
		p.Log.Info(fmt.Sprintf("iso: removed %s from proxmox host", isoBase))
	}

	return nil
}

// parseNullDelimitedFileList splits find -print0 output on null bytes, which
// is unambiguous even when filenames contain newlines or spaces.
func parseNullDelimitedFileList(output string) []string {
	var files []string
	for _, entry := range strings.Split(output, "\x00") {
		entry = strings.TrimSpace(entry)
		if entry != "" {
			files = append(files, entry)
		}
	}
	return files
}
