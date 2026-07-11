package hostssh

import (
	"context"
	"fmt"
	"strconv"
)

// pveshRun executes a pvesh subcommand on the Proxmox host in argv mode so
// no shell string is assembled from user-controlled atoms. p.Node is
// validated once here; callers must not re-validate.
func pveshRun(ctx context.Context, p *RemoteISOParams, subcommand, path string) (string, error) {
	if err := validateProxmoxName(p.Node); err != nil {
		return "", fmt.Errorf("proxmox node %q invalid: %w", p.Node, err)
	}
	result, err := SSHRunArgvOutput(ctx, p.Exec, p.Host, p.KnownHostsPath,
		"pvesh", subcommand, path, "--output-format", "json",
	)
	if err != nil {
		return "", err
	}
	if result.Truncated {
		return "", fmt.Errorf("pvesh %s %s output truncated after %d bytes", subcommand, path, len(result.Stdout))
	}
	return result.Stdout, nil
}

// PveshRun is the exported entry point for callers outside package hostssh.
// It inherits pveshRun's validateProxmoxName guard, so callers must not
// validate p.Node themselves. Returns the raw JSON stdout on success.
func PveshRun(ctx context.Context, p *RemoteISOParams, subcommand, path string) (string, error) {
	return pveshRun(ctx, p, subcommand, path)
}

func pveshQEMUPath(node string) string {
	return "/nodes/" + node + "/qemu"
}

func pveshConfigPath(node string, vmid int) string {
	return "/nodes/" + node + "/qemu/" + strconv.Itoa(vmid) + "/config"
}
