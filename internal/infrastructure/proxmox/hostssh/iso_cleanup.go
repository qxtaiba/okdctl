package hostssh

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

// DefaultProxmoxISODir is Proxmox's default ISO path, populated via scp and
// referenced by `qm importdisk`.
const DefaultProxmoxISODir = "/var/lib/vz/template/iso"

// RemoteISOParams carries the shared connection parameters for pvesh/ssh
// operations against a Proxmox host. Host must be a bare hostname or IP (no
// port); an empty KnownHostsPath allows accept-new TOFU, otherwise strict
// host-key checking applies.
type RemoteISOParams struct {
	Host           string
	Node           string
	Exec           *executor.Executor
	Log            *slog.Logger
	KnownHostsPath string
}

// refuseUnsafeISOPath rejects any path outside <isoDir>/<name> matching a
// nodetypes.CoreOSISONamePatterns entry, guarding against a config typo
// pointing an SSH rm at an arbitrary host path.
func refuseUnsafeISOPath(isoDir, path string) error {
	cleaned := filepath.Clean(path)
	dir := filepath.Dir(cleaned)
	base := filepath.Base(cleaned)

	if dir != filepath.Clean(isoDir) {
		return fmt.Errorf("refusing unsafe remote path %q: not inside %s", path, isoDir)
	}
	if !nodetypes.IsCoreOSISOName(base) {
		return fmt.Errorf("refusing unsafe remote path %q: not a recognized base coreos iso filename", path)
	}
	return nil
}

// shellSingleQuote POSIX-quotes s so it's safe as a literal remote shell
// command-string argument.
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// validateProxmoxName is the defense-in-depth guard at the pveshRun
// boundary, catching a hand-edited YAML that bypasses
// config.ValidateOKDConfig; new pvesh callers must route through pveshRun
// rather than interpolating names directly.
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

// ValidateISODir rejects isoDir values that contain shell metacharacters or
// are not absolute paths.
func ValidateISODir(isoDir string) error {
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

// ValidateRemoteFilename rejects filenames with shell metacharacters, path
// separators, or the ".." traversal atom. A hostile filesystem entry could
// otherwise inject commands into the remote login shell via an SSHRunArgv
// argument.
func ValidateRemoteFilename(name string) error {
	if name == "" {
		return fmt.Errorf("remote filename must not be empty")
	}
	if name == ".." || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("remote filename %q must be a plain filename (no path separators or traversal)", name)
	}
	const dangerous = ";|&$`\\\"!(){}<>~*?[]# \t\n\r'"
	for _, r := range dangerous {
		if strings.ContainsRune(name, r) {
			return fmt.Errorf("remote filename %q contains unsafe character %q", name, string(r))
		}
	}
	return nil
}

// parseVMIDsFromSummary returns running VMs' vmids only — yanking a cdrom
// disrupts a running VM, but a stopped one referencing an ISO can be
// destroyed cleanly.
func parseVMIDsFromSummary(data []byte) ([]int, error) {
	var vms []struct {
		VMID   int               `json:"vmid"`
		Status nodetypes.VMState `json:"status"`
	}
	if err := json.Unmarshal(data, &vms); err != nil {
		return nil, fmt.Errorf("pvesh qemu list output not valid json: %w", err)
	}
	var ids []int
	for _, vm := range vms {
		if vm.Status == nodetypes.StateRunning {
			ids = append(ids, vm.VMID)
		}
	}
	return ids, nil
}

func configDevicesReferenceISO(data []byte, isoBase string) (bool, error) {
	var config map[string]json.RawMessage
	if err := json.Unmarshal(data, &config); err != nil {
		return false, fmt.Errorf("pvesh config output not valid json: %w", err)
	}
	return vmDevicesReferenceISO(config, isoBase), nil
}

// vmConfigReferencesISO fails closed (returns true) if the config fetch
// errors, so an ISO of unknown usage is never removed.
func vmConfigReferencesISO(ctx context.Context, p *RemoteISOParams, vmid int, isoBase string) (bool, error) {
	result, err := pveshRun(ctx, p, "get", pveshConfigPath(p.Node, vmid))
	if err != nil {
		return true, fmt.Errorf("ssh pvesh qemu config for vmid %d: %w", vmid, err)
	}
	found, parseErr := configDevicesReferenceISO([]byte(result), isoBase)
	if parseErr != nil {
		return true, fmt.Errorf("vmid %d: %w", vmid, parseErr)
	}
	return found, nil
}

func anyVMReferencesISO(ctx context.Context, p *RemoteISOParams, isoBase string) (bool, error) {
	result, err := pveshRun(ctx, p, "get", pveshQEMUPath(p.Node))
	if err != nil {
		return false, fmt.Errorf("ssh pvesh qemu list: %w", err)
	}
	vmids, err := parseVMIDsFromSummary([]byte(result))
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

// vmDevicesReferenceISO matches when any device field's comma-separated
// segment ends with isoBase.
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
		// Device entries may be bare paths or "file=<storage>:<pool>/<name>".
		for seg := range strings.SplitSeq(value, ",") {
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

// findCoreOSISONameClause builds a find(1) -name test from
// nodetypes.CoreOSISONamePatterns so it can't drift from
// refuseUnsafeISOPath's allowlist.
func findCoreOSISONameClause() string {
	terms := make([]string, len(nodetypes.CoreOSISONamePatterns))
	for i, pat := range nodetypes.CoreOSISONamePatterns {
		terms[i] = "-name " + shellSingleQuote(pat)
	}
	return `\( ` + strings.Join(terms, " -o ") + ` \)`
}

// RemoveFCOSISOFromProxmox removes base CoreOS ISOs (nodetypes.
// CoreOSISONamePatterns) from isoDir over SSH, skipping any still
// referenced by a running VM.
//
// Shell-injection policy: this is the repo's only sh -c call site (via
// sshRun); every other SSH operation MUST use SSHRunArgv. New sh -c usage
// MUST add its own validateXxx guard and shellSingleQuote every variable
// token before interpolation.
func RemoveFCOSISOFromProxmox(ctx context.Context, p *RemoteISOParams, isoDir string) error {
	if err := ValidateISODir(isoDir); err != nil {
		return err
	}

	findCmd := fmt.Sprintf(
		"find %s -maxdepth 1 %s -type f -print0 2>/dev/null || true",
		shellSingleQuote(isoDir), findCoreOSISONameClause(),
	)
	result, err := sshRunOutput(ctx, p.Exec, p.Host, p.KnownHostsPath, findCmd)
	if err != nil {
		return fmt.Errorf("ssh find: %w", err)
	}
	if result.Truncated {
		return fmt.Errorf("ssh find output truncated after %d bytes; refusing to process a partial file list", len(result.Stdout))
	}

	files := parseNullDelimitedFileList(result.Stdout)
	if len(files) == 0 {
		p.Log.Info("iso: no base coreos iso found on proxmox host, nothing to remove")
		return nil
	}

	for _, f := range files {
		if err := refuseUnsafeISOPath(isoDir, f); err != nil {
			p.Log.Warn("iso: skipping file", "path", f, "err", err)
			continue
		}

		isoBase := filepath.Base(f)
		// Full "iso/<file>" token avoids aliasing two same-named ISOs across storage layouts.
		inUse, err := anyVMReferencesISO(ctx, p, "iso/"+isoBase)
		if err != nil {
			p.Log.Warn("iso: could not check vm references — skipping", "file", isoBase, "err", err)
			continue
		}
		if inUse {
			p.Log.Warn("iso: still referenced by a running vm — skipping removal", "file", isoBase)
			continue
		}

		// shellSingleQuote keeps spaces/metacharacters in f as one rm argument.
		if _, rmErr := sshRun(ctx, p.Exec, p.Host, p.KnownHostsPath, "rm -f "+shellSingleQuote(f)); rmErr != nil {
			p.Log.Warn("iso: failed to remove", "file", isoBase, "err", rmErr)
			continue
		}
		p.Log.Info("iso: removed from proxmox host", "file", isoBase)
	}

	return nil
}

// RemoveCustomISOsFromProxmox removes the exact per-node ISOs built by
// provision.BuildNodeList from isoDir via SSHRunArgv (exact names, no
// glob). A name that fails validation or is still referenced by a running
// VM is skipped with a warning, not aborted.
func RemoveCustomISOsFromProxmox(ctx context.Context, p *RemoteISOParams, isoDir string, names []string) error {
	if err := ValidateISODir(isoDir); err != nil {
		return err
	}

	for _, name := range names {
		if err := ValidateRemoteFilename(name); err != nil {
			p.Log.Warn("iso: skipping custom iso with unsafe filename", "file", name, "err", err)
			continue
		}

		inUse, err := anyVMReferencesISO(ctx, p, "iso/"+name)
		if err != nil {
			p.Log.Warn("iso: could not check vm references — skipping", "file", name, "err", err)
			continue
		}
		if inUse {
			p.Log.Warn("iso: still referenced by a running vm — skipping removal", "file", name)
			continue
		}

		target := isoDir + "/" + name
		if _, rmErr := SSHRunArgv(ctx, p.Exec, p.Host, p.KnownHostsPath, "rm", "-f", "--", target); rmErr != nil {
			p.Log.Warn("iso: failed to remove", "file", name, "err", rmErr)
			continue
		}
		p.Log.Info("iso: removed custom node iso from proxmox host", "file", name)
	}

	return nil
}

// parseNullDelimitedFileList splits find -print0 output on null bytes —
// unambiguous even with spaces/newlines in filenames.
func parseNullDelimitedFileList(output string) []string {
	var files []string
	for entry := range strings.SplitSeq(output, "\x00") {
		entry = strings.TrimSpace(entry)
		if entry != "" {
			files = append(files, entry)
		}
	}
	return files
}
