package phase

import (
	"context"
	"fmt"
	"strconv"
)

// pveshRun executes a pvesh subcommand on the Proxmox host in argv mode so
// no shell string is assembled from user-controlled atoms. p.Node is
// validated once here; callers must not re-validate.
func pveshRun(ctx context.Context, p *RemoteISOParams, subcommand, path string) (*pveshResult, error) {
	if err := validateProxmoxName(p.Node); err != nil {
		return nil, fmt.Errorf("proxmox node %q invalid: %w", p.Node, err)
	}
	result, err := SSHRunArgv(ctx, p.Exec, p.Host, p.KnownHostsPath,
		"pvesh", subcommand, path, "--output-format", "json",
	)
	if err != nil {
		return nil, err
	}
	return &pveshResult{stdout: result.Stdout}, nil
}

// PveshRun is the exported entry point for callers outside package phase.
// It inherits pveshRun's validateProxmoxName guard, so callers must not
// validate p.Node themselves. Returns the raw JSON stdout on success.
//
// Lives in phase/ rather than infrastructure/proxmox to avoid an import
// cycle: proxmox imports phase (for VMState, NodeRole), so pvesh helpers
// cannot live in proxmox without cycling back. infrastructure/proxmox is
// the sole direct consumer. api:7f2bf677 — if iso_cleanup moves out of
// phase/, pvesh helpers follow.
func PveshRun(ctx context.Context, p *RemoteISOParams, subcommand, path string) (string, error) {
	result, err := pveshRun(ctx, p, subcommand, path)
	if err != nil {
		return "", err
	}
	return result.stdout, nil
}

type pveshResult struct {
	stdout string
}

func pveshQEMUPath(node string) string {
	return "/nodes/" + node + "/qemu"
}

func pveshConfigPath(node string, vmid int) string {
	return "/nodes/" + node + "/qemu/" + strconv.Itoa(vmid) + "/config"
}
