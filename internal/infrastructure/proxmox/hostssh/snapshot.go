package hostssh

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/system"
)

// SnapshotInfo describes one QEMU snapshot as returned by
// pvesh get /nodes/<node>/qemu/<vmid>/snapshot. ListSnapshots filters out
// the synthetic "current" entry Proxmox uses to anchor its snapshot tree —
// it is not a real, rollback-able or deletable snapshot.
type SnapshotInfo struct {
	Name        string
	Description string
	SnapTime    int64
	Parent      string
}

// taskStatusStopped is the terminal Status value pveshWaitTask polls for;
// ExitStatus then distinguishes a clean finish ("OK") from a failure.
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

// validateSnapshotName enforces the pve-configid grammar Proxmox itself
// requires for a snapshot name. This doubles as the shell-injection guard
// for every path built with the name (SSHRunArgv does not sanitize argv
// atoms against the remote login shell — see ssh.go).
func validateSnapshotName(name string) error {
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

// validateSnapshotDescription allowlist-checks an optional free-text
// description before it reaches the remote shell as an SSHRunArgv atom.
// Whitespace is rejected, not just control characters: SSHRunArgv joins argv
// with spaces before handing it to the remote login shell, so a multi-word
// value would word-split there instead of surviving as the single token
// pvesh expects. An empty description is valid — CreateSnapshot omits
// -description entirely when desc is "".
func validateSnapshotDescription(desc string) error {
	if len(desc) > 200 {
		return fmt.Errorf("must be 200 characters or fewer, got %d", len(desc))
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

// validateUPID rejects UPID values outside the Proxmox task-id charset,
// guarding pveshWaitTask's status-path interpolation the same way
// validateSnapshotName guards create/rollback/delete.
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

// pveshTaskCall issues a pvesh subcommand that launches an async background
// task (snapshot create/rollback/delete) and returns its UPID. pveshRun
// tolerates a non-zero exit because its callers are read paths; a task
// launch must fail loudly on rejection instead of returning whatever ended
// up on stdout, so this checks result.ExitCode itself rather than going
// through pveshRun.
func pveshTaskCall(ctx context.Context, p *RemoteISOParams, subcommand, path string, extra ...string) (string, error) {
	if err := validateProxmoxName(p.Node); err != nil {
		return "", fmt.Errorf("proxmox node %q invalid: %w", p.Node, err)
	}
	argv := append([]string{"pvesh", subcommand, path}, extra...)
	argv = append(argv, "--output-format", "json")
	result, err := SSHRunArgvOutput(ctx, p.Exec, p.Host, p.KnownHostsPath, argv...)
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("pvesh %s %s: exit %d: %s", subcommand, path, result.ExitCode, strings.TrimSpace(result.Stderr))
	}
	if result.Truncated {
		return "", fmt.Errorf("pvesh %s %s output truncated after %d bytes", subcommand, path, len(result.Stdout))
	}
	return result.Stdout, nil
}

func pveshCreateTaskCall(ctx context.Context, p *RemoteISOParams, path string, extra ...string) (string, error) {
	stdout, err := pveshTaskCall(ctx, p, "create", path, extra...)
	if err != nil {
		return "", err
	}
	return parseUPID(stdout)
}

func pveshDeleteTaskCall(ctx context.Context, p *RemoteISOParams, path string, extra ...string) (string, error) {
	stdout, err := pveshTaskCall(ctx, p, "delete", path, extra...)
	if err != nil {
		return "", err
	}
	return parseUPID(stdout)
}

// parseUPID decodes the JSON string a pvesh create/delete task call returns
// and validates it before any caller interpolates it into a status-path
// lookup.
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

// pveshWaitTask polls a pvesh background task until it reaches
// status "stopped", then checks exitstatus. A stopped task with a
// non-OK exitstatus is reported as a distinct error rather than folded
// into a generic timeout, since the two failure modes need different
// operator responses (task rejected outright vs. never finished).
func pveshWaitTask(ctx context.Context, p *RemoteISOParams, upid string, timeout time.Duration) error {
	if err := validateUPID(upid); err != nil {
		return fmt.Errorf("upid %q invalid: %w", upid, err)
	}

	var status taskStatus
	var parseErr error
	check := func(ctx context.Context) bool {
		stdout, err := pveshRun(ctx, p, "get", pveshTaskStatusPath(p.Node, upid))
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

// CreateSnapshot snapshots vmid's disks and, if description is non-empty,
// attaches it to the snapshot. It never passes -vmstate: the qemu-guest-agent
// is disabled fleet-wide, so no in-VM freeze is available and a memory-state
// snapshot would not be crash-consistent with it. Blocks until the async
// pvesh task completes or timeout elapses.
func CreateSnapshot(ctx context.Context, p *RemoteISOParams, vmid int, name, description string, timeout time.Duration) error {
	if err := validateVMID(vmid); err != nil {
		return fmt.Errorf("vmid %d invalid: %w", vmid, err)
	}
	if err := validateSnapshotName(name); err != nil {
		return fmt.Errorf("snapshot name %q invalid: %w", name, err)
	}
	if err := validateSnapshotDescription(description); err != nil {
		return fmt.Errorf("snapshot description invalid: %w", err)
	}

	extra := []string{"-snapname", name}
	if description != "" {
		extra = append(extra, "-description", description)
	}
	upid, err := pveshCreateTaskCall(ctx, p, pveshSnapshotPath(p.Node, vmid), extra...)
	if err != nil {
		return fmt.Errorf("create snapshot %s for vmid %d: %w", name, vmid, err)
	}
	if err := pveshWaitTask(ctx, p, upid, timeout); err != nil {
		return fmt.Errorf("wait for snapshot %s create task: %w", name, err)
	}
	return nil
}

// ListSnapshots returns vmid's snapshots in the order pvesh reports them;
// Proxmox does not document that order as chronological.
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

// RollbackSnapshot restores vmid's disks to name and passes -start 1, which
// auto-starts the VM once the rollback completes — including a VM that was
// deliberately powered off beforehand. Blocks until the async pvesh task
// completes or timeout elapses.
func RollbackSnapshot(ctx context.Context, p *RemoteISOParams, vmid int, name string, timeout time.Duration) error {
	if err := validateVMID(vmid); err != nil {
		return fmt.Errorf("vmid %d invalid: %w", vmid, err)
	}
	if err := validateSnapshotName(name); err != nil {
		return fmt.Errorf("snapshot name %q invalid: %w", name, err)
	}

	path := pveshSnapshotNamePath(p.Node, vmid, name) + "/rollback"
	upid, err := pveshCreateTaskCall(ctx, p, path, "-start", "1")
	if err != nil {
		return fmt.Errorf("rollback vmid %d to snapshot %s: %w", vmid, name, err)
	}
	if err := pveshWaitTask(ctx, p, upid, timeout); err != nil {
		return fmt.Errorf("wait for snapshot %s rollback task: %w", name, err)
	}
	return nil
}

// DeleteSnapshot removes name from vmid. Blocks until the async pvesh task
// completes or timeout elapses.
func DeleteSnapshot(ctx context.Context, p *RemoteISOParams, vmid int, name string, timeout time.Duration) error {
	if err := validateVMID(vmid); err != nil {
		return fmt.Errorf("vmid %d invalid: %w", vmid, err)
	}
	if err := validateSnapshotName(name); err != nil {
		return fmt.Errorf("snapshot name %q invalid: %w", name, err)
	}

	upid, err := pveshDeleteTaskCall(ctx, p, pveshSnapshotNamePath(p.Node, vmid, name))
	if err != nil {
		return fmt.Errorf("delete snapshot %s for vmid %d: %w", name, vmid, err)
	}
	if err := pveshWaitTask(ctx, p, upid, timeout); err != nil {
		return fmt.Errorf("wait for snapshot %s delete task: %w", name, err)
	}
	return nil
}

// VMAgentEnabled probes vmid's live config for the qemu-guest-agent flag
// rather than assuming it fleet-wide, so a future per-VM opt-in is picked
// up without a code change. Callers use this to decide whether a crash-
// consistency warning applies to a given snapshot.
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

// agentFlagEnabled parses the Proxmox qemu-agent config value, which is
// either a bare "0"/"1" or a comma-separated property string whose first
// segment is the enable flag (e.g. "enabled=1,fstrim_cloned_disks=1").
func agentFlagEnabled(raw string) bool {
	first, _, _ := strings.Cut(raw, ",")
	if v, ok := strings.CutPrefix(first, "enabled="); ok {
		return v == "1"
	}
	return first == "1"
}
