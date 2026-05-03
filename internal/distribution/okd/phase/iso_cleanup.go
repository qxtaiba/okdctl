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

// validateProxmoxName is the defense-in-depth guard centralized at the
// pveshRun helper boundary; config.ValidateOKDConfig already rejects
// malformed node / storage names at load time, but a hand-edited YAML
// could otherwise reach the remote-shell path through okdctl destroy.
// New pvesh callers go through pveshRun and inherit this guard
// automatically — do not interpolate names into ssh command strings
// without it.
func validateProxmoxName(name string) error {
	if name == "" {
		return fmt.Errorf("must not be empty")
	}
	for i, r := range name {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_'
		if !ok {
			return fmt.Errorf("character %q at position %d not in [A-Za-z0-9_-]", string(r), i)
		}
	}
	return nil
}

// validateISODir rejects isoDir values that contain shell metacharacters or
// are not absolute paths.
func validateISODir(isoDir string) error {
	if !filepath.IsAbs(isoDir) {
		return fmt.Errorf("isoDir must be an absolute path, got %q", isoDir)
	}
	const dangerous = ";|&$`\\\"!(){}<>~*?[]# \t\n\r'"
	for _, r := range dangerous {
		if strings.ContainsRune(isoDir, r) {
			return fmt.Errorf("isoDir %q contains unsafe character %q", isoDir, string(r))
		}
	}
	return nil
}

// parseVMIDsFromSummary parses the JSON array returned by
// pvesh get /nodes/<node>/qemu and returns the vmid of each running VM.
// Stopped VMs are excluded: yanking a cdrom from a running VM disrupts it,
// but a stopped VM that still references an ISO can be destroyed cleanly.
func parseVMIDsFromSummary(data []byte) ([]int, error) {
	var vms []struct {
		VMID   int     `json:"vmid"`
		Status VMState `json:"status"`
	}
	if err := json.Unmarshal(data, &vms); err != nil {
		return nil, fmt.Errorf("pvesh qemu list output not valid json: %w", err)
	}
	var ids []int
	for _, vm := range vms {
		if vm.Status == StateRunning {
			ids = append(ids, vm.VMID)
		}
	}
	return ids, nil
}

// configDevicesReferenceISO parses the JSON object returned by
// pvesh get /nodes/<node>/qemu/<vmid>/config and returns true if any
// device-mapping field references isoBase.
func configDevicesReferenceISO(data []byte, isoBase string) (bool, error) {
	var config map[string]json.RawMessage
	if err := json.Unmarshal(data, &config); err != nil {
		return false, fmt.Errorf("pvesh config output not valid json: %w", err)
	}
	return vmDevicesReferenceISO(config, isoBase), nil
}

func listProxmoxVMIDs(ctx context.Context, p *RemoteISOParams) ([]int, error) {
	result, err := pveshRun(ctx, p, "get", pveshQEMUPath(p.Node))
	if err != nil {
		return nil, fmt.Errorf("ssh pvesh qemu list failed: %w", err)
	}
	return parseVMIDsFromSummary([]byte(result.stdout))
}

// vmConfigReferencesISO fetches the per-VM config for vmid and returns true
// if any device-mapping field references isoBase. If the config call fails,
// it returns true (fail-closed) to prevent removing an ISO whose usage is unknown.
func vmConfigReferencesISO(ctx context.Context, p *RemoteISOParams, vmid int, isoBase string) (bool, error) {
	result, err := pveshRun(ctx, p, "get", pveshConfigPath(p.Node, vmid))
	if err != nil {
		return true, fmt.Errorf("ssh pvesh qemu config failed for vmid %d: %w", vmid, err)
	}
	found, parseErr := configDevicesReferenceISO([]byte(result.stdout), isoBase)
	if parseErr != nil {
		return true, fmt.Errorf("vmid %d: %w", vmid, parseErr)
	}
	return found, nil
}

// anyVMReferencesISO returns true if any running VM on the node has isoBase
// in its device-mapping configuration. Issues one summary call and one config
// call per running VM.
func anyVMReferencesISO(ctx context.Context, p *RemoteISOParams, isoBase string) (bool, error) {
	vmids, err := listProxmoxVMIDs(ctx, p)
	if err != nil {
		return false, err
	}
	for _, vmid := range vmids {
		referenced, err := vmConfigReferencesISO(ctx, p, vmid, isoBase)
		if err != nil {
			return true, err
		}
		if referenced {
			return true, nil
		}
	}
	return false, nil
}

var deviceFields = func() []string {
	fields := []string{"boot", "bootdisk"}
	for i := range 4 {
		fields = append(fields, fmt.Sprintf("ide%d", i))
	}
	for i := range 6 {
		fields = append(fields, fmt.Sprintf("sata%d", i))
	}
	for i := range 31 {
		fields = append(fields, fmt.Sprintf("scsi%d", i))
	}
	for i := range 16 {
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

	findCmd := fmt.Sprintf(
		"find %s -maxdepth 1 -name 'fedora-coreos-*.iso' -type f -print0 2>/dev/null || true",
		shellSingleQuote(isoDir),
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
			p.Log.Warn("iso: skipping file", "file", f, "err", err)
			continue
		}

		isoBase := filepath.Base(f)
		inUse, err := anyVMReferencesISO(ctx, p, isoBase)
		if err != nil {
			p.Log.Warn("iso: could not check vm references — skipping", "iso", isoBase, "err", err)
			continue
		}
		if inUse {
			p.Log.Warn(fmt.Sprintf("iso: %s is still referenced by a running vm — skipping removal", isoBase))
			continue
		}

		// Shell-single-quote the path so filenames with spaces or metacharacters
		// reach rm as a single literal argument.
		if _, rmErr := SSHRun(ctx, p.Exec, p.Host, "rm -f "+shellSingleQuote(f)); rmErr != nil {
			p.Log.Warn("iso: failed to remove", "iso", isoBase, "err", rmErr)
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
