package hostssh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/system"
)

// errSnapshotExists reports a duplicate snapshot name; CreateSnapshot wraps
// it via errors.Is instead of surfacing the raw pvesh exitstatus.
var errSnapshotExists = errors.New("snapshot already exists")

// SnapshotInfo describes one QEMU snapshot; ListSnapshots filters out
// Proxmox's synthetic "current" anchor entry, which is not a real,
// deletable snapshot.
type SnapshotInfo struct {
	Name        string
	Description string
	SnapTime    int64
	Parent      string
}

// taskStatusStopped is the terminal Status pveshWaitTask polls for; ExitStatus then says OK or not.
const taskStatusStopped = "stopped"

// taskStatus is the shape of pvesh get /nodes/<node>/tasks/<upid>/status.
type taskStatus struct {
	Status     string `json:"status"`
	ExitStatus string `json:"exitstatus"`
}

func pveshSnapshotPath(node string, vmid int) string {
	return "/nodes/" + node + "/qemu/" + strconv.Itoa(vmid) + "/snapshot"
}

func pveshSnapshotNamePath(node string, vmid int, name string) string {
	return pveshSnapshotPath(node, vmid) + "/" + name
}

func pveshTaskStatusPath(node, upid string) string {
	return "/nodes/" + node + "/tasks/" + upid + "/status"
}

// validateVMID rejects vmid values outside the Proxmox-assignable range.
func validateVMID(vmid int) error {
	if vmid < 100 || vmid > 999999999 {
		return fmt.Errorf("vmid %d out of range [100, 999999999]", vmid)
	}
	return nil
}

// ValidateSnapshotName enforces Proxmox's pve-configid grammar and is the
// authoritative shell-injection guard for snapshot-name paths; SSHRunArgv's
// atom check (ssh.go) is only a fail-closed backstop.
func ValidateSnapshotName(name string) error {
	if name == "" {
		return fmt.Errorf("must not be empty")
	}
	if len(name) > 40 {
		return fmt.Errorf("must be 40 characters or fewer, got %d", len(name))
	}
	first := rune(name[0])
	isLetter := (first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z')
	if !isLetter {
		return fmt.Errorf("must start with a letter, got %q", string(first))
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

// ValidateSnapshotDescription allowlist-checks an optional description
// before it becomes an SSHRunArgv atom; whitespace is rejected because
// SSHRunArgv's space-join would word-split a multi-word value before pvesh
// sees it. Empty is valid: CreateSnapshot omits -description then.
func ValidateSnapshotDescription(desc string) error {
	if len(desc) > 200 {
		return fmt.Errorf("must be 200 characters or fewer, got %d", len(desc))
	}
	// Letter/digit-first prevents desc from reading as a pvesh option flag (e.g. -vmstate).
	if desc != "" {
		first := rune(desc[0])
		ok := (first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z') || (first >= '0' && first <= '9')
		if !ok {
			return fmt.Errorf("must start with a letter or digit, got %q", string(first))
		}
	}
	for i, r := range desc {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '.' || r == ',' || r == ':' || r == '_' || r == '-'
		if !ok {
			return fmt.Errorf("character %q at position %d not in [A-Za-z0-9.,:_-] (no whitespace — must be a single token; use dashes or underscores)", string(r), i)
		}
	}
	return nil
}

// validateUPID guards pveshWaitTask's status-path interpolation the same
// way ValidateSnapshotName guards create/rollback/delete.
func validateUPID(upid string) error {
	if upid == "" {
		return fmt.Errorf("must not be empty")
	}
	for i, r := range upid {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == ':' || r == '@' || r == '.' || r == '_' || r == '-'
		if !ok {
			return fmt.Errorf("character %q at position %d not in [A-Za-z0-9:@._-]", string(r), i)
		}
	}
	return nil
}

// pveshTaskUPID launches an async pvesh task (snapshot create/rollback/
// delete) and returns its UPID. It routes through pveshRunChecked, not
// pveshRun, so a rejected launch fails loudly instead of returning whatever
// landed on stdout.
func pveshTaskUPID(ctx context.Context, p *RemoteISOParams, subcommand, path string, extra ...string) (string, error) {
	stdout, err := pveshRunChecked(ctx, p, subcommand, path, extra...)
	if err != nil {
		return "", err
	}
	return parseUPID(stdout)
}

// parseUPID decodes and validates the UPID pvesh returns before any caller
// interpolates it into a status-path lookup.
func parseUPID(stdout string) (string, error) {
	var upid string
	if err := json.Unmarshal([]byte(stdout), &upid); err != nil {
		return "", fmt.Errorf("pvesh task output not a json string: %w", err)
	}
	if err := validateUPID(upid); err != nil {
		return "", fmt.Errorf("upid %q invalid: %w", upid, err)
	}
	return upid, nil
}

// pveshWaitTask polls until status reaches "stopped", then checks
// exitstatus. A non-OK exitstatus is reported as its own error rather than
// folded into a timeout, since the two failure modes need different
// operator responses.
func pveshWaitTask(ctx context.Context, p *RemoteISOParams, upid string, timeout time.Duration) error {
	if err := validateUPID(upid); err != nil {
		return fmt.Errorf("upid %q invalid: %w", upid, err)
	}

	var status taskStatus
	var parseErr error
	check := func(ctx context.Context) bool {
		stdout, err := pveshRunChecked(ctx, p, "get", pveshTaskStatusPath(p.Node, upid))
		if err != nil {
			parseErr = err
			return false
		}
		var s taskStatus
		if err := json.Unmarshal([]byte(stdout), &s); err != nil {
			parseErr = err
			return false
		}
		status, parseErr = s, nil
		return s.Status == taskStatusStopped
	}

	if err := system.WaitForWithTimeout(ctx, "pvesh task", upid, check, timeout, p.Log); err != nil {
		return err
	}
	if parseErr != nil {
		return fmt.Errorf("pvesh task %s status: %w", upid, parseErr)
	}
	if status.ExitStatus != "OK" {
		return fmt.Errorf("pvesh task %s finished with exitstatus %q", upid, status.ExitStatus)
	}
	return nil
}

// CreateSnapshot snapshots vmid's disks (attaching description if
// non-empty) and blocks until the task completes or timeout elapses. It
// never passes -vmstate — qemu-guest-agent is disabled fleet-wide, so a
// memory-state snapshot wouldn't be crash-consistent — and returns an error
// matching errSnapshotExists (errors.Is) if name already exists.
func CreateSnapshot(ctx context.Context, p *RemoteISOParams, vmid int, name, description string, timeout time.Duration) error {
	if err := validateVMID(vmid); err != nil {
		return fmt.Errorf("vmid %d invalid: %w", vmid, err)
	}
	if err := ValidateSnapshotName(name); err != nil {
		return fmt.Errorf("snapshot name %q invalid: %w", name, err)
	}
	if err := ValidateSnapshotDescription(description); err != nil {
		return fmt.Errorf("snapshot description invalid: %w", err)
	}

	// Best-effort pre-check: a definite collision surfaces as errSnapshotExists;
	// a failed listing is ignored since pvesh itself rejects duplicates and
	// this check is racy anyway.
	if existing, listErr := ListSnapshots(ctx, p, vmid); listErr == nil {
		for _, s := range existing {
			if s.Name == name {
				return fmt.Errorf("snapshot %q for vmid %d: %w", name, vmid, errSnapshotExists)
			}
		}
	}

	extra := []string{"-snapname", name}
	if description != "" {
		extra = append(extra, "-description", description)
	}
	upid, err := pveshTaskUPID(ctx, p, "create", pveshSnapshotPath(p.Node, vmid), extra...)
	if err != nil {
		return fmt.Errorf("create snapshot %s for vmid %d: %w", name, vmid, err)
	}
	if err := pveshWaitTask(ctx, p, upid, timeout); err != nil {
		return fmt.Errorf("wait for snapshot %s create task: %w", name, err)
	}
	return nil
}

// ListSnapshots returns vmid's snapshots in pvesh's reported order, which
// Proxmox does not document as chronological.
func ListSnapshots(ctx context.Context, p *RemoteISOParams, vmid int) ([]SnapshotInfo, error) {
	if err := validateVMID(vmid); err != nil {
		return nil, fmt.Errorf("vmid %d invalid: %w", vmid, err)
	}
	stdout, err := pveshRun(ctx, p, "get", pveshSnapshotPath(p.Node, vmid))
	if err != nil {
		return nil, fmt.Errorf("list snapshots for vmid %d: %w", vmid, err)
	}
	return parseSnapshotList(stdout)
}

func parseSnapshotList(stdout string) ([]SnapshotInfo, error) {
	var raw []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		SnapTime    int64  `json:"snaptime"`
		Parent      string `json:"parent"`
	}
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		return nil, fmt.Errorf("pvesh snapshot list output not valid json: %w", err)
	}
	snapshots := make([]SnapshotInfo, 0, len(raw))
	for _, s := range raw {
		if s.Name == "current" {
			continue
		}
		snapshots = append(snapshots, SnapshotInfo{
			Name:        s.Name,
			Description: s.Description,
			SnapTime:    s.SnapTime,
			Parent:      s.Parent,
		})
	}
	return snapshots, nil
}

// RollbackSnapshot restores vmid's disks to name, passing -start 1 so the
// VM auto-starts after rollback (even if deliberately powered off), and
// blocks until the task completes or timeout elapses.
func RollbackSnapshot(ctx context.Context, p *RemoteISOParams, vmid int, name string, timeout time.Duration) error {
	if err := validateVMID(vmid); err != nil {
		return fmt.Errorf("vmid %d invalid: %w", vmid, err)
	}
	if err := ValidateSnapshotName(name); err != nil {
		return fmt.Errorf("snapshot name %q invalid: %w", name, err)
	}

	path := pveshSnapshotNamePath(p.Node, vmid, name) + "/rollback"
	upid, err := pveshTaskUPID(ctx, p, "create", path, "-start", "1")
	if err != nil {
		return fmt.Errorf("rollback vmid %d to snapshot %s: %w", vmid, name, err)
	}
	if err := pveshWaitTask(ctx, p, upid, timeout); err != nil {
		return fmt.Errorf("wait for snapshot %s rollback task: %w", name, err)
	}
	return nil
}

// DeleteSnapshot removes name from vmid, blocking until the task completes or timeout elapses.
func DeleteSnapshot(ctx context.Context, p *RemoteISOParams, vmid int, name string, timeout time.Duration) error {
	if err := validateVMID(vmid); err != nil {
		return fmt.Errorf("vmid %d invalid: %w", vmid, err)
	}
	if err := ValidateSnapshotName(name); err != nil {
		return fmt.Errorf("snapshot name %q invalid: %w", name, err)
	}

	upid, err := pveshTaskUPID(ctx, p, "delete", pveshSnapshotNamePath(p.Node, vmid, name))
	if err != nil {
		return fmt.Errorf("delete snapshot %s for vmid %d: %w", name, vmid, err)
	}
	if err := pveshWaitTask(ctx, p, upid, timeout); err != nil {
		return fmt.Errorf("wait for snapshot %s delete task: %w", name, err)
	}
	return nil
}

// VMAgentEnabled probes vmid's live config for the qemu-guest-agent flag
// rather than assuming it fleet-wide, so a future per-VM opt-in needs no
// code change.
func VMAgentEnabled(ctx context.Context, p *RemoteISOParams, vmid int) (bool, error) {
	if err := validateVMID(vmid); err != nil {
		return false, fmt.Errorf("vmid %d invalid: %w", vmid, err)
	}
	stdout, err := pveshRun(ctx, p, "get", pveshConfigPath(p.Node, vmid))
	if err != nil {
		return false, fmt.Errorf("get vm config for vmid %d: %w", vmid, err)
	}
	var cfg struct {
		Agent string `json:"agent"`
	}
	if err := json.Unmarshal([]byte(stdout), &cfg); err != nil {
		return false, fmt.Errorf("vm config for vmid %d not valid json: %w", vmid, err)
	}
	return agentFlagEnabled(cfg.Agent), nil
}

// agentFlagEnabled parses Proxmox's qemu-agent value: bare "0"/"1" or a
// comma-separated property string whose first segment is the flag.
func agentFlagEnabled(raw string) bool {
	first, _, _ := strings.Cut(raw, ",")
	if v, ok := strings.CutPrefix(first, "enabled="); ok {
		return v == "1"
	}
	return first == "1"
}
