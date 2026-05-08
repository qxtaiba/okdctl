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

type pveshResult struct {
	stdout string
}

func pveshQEMUPath(node string) string {
	return "/nodes/" + node + "/qemu"
}

func pveshConfigPath(node string, vmid int) string {
	return "/nodes/" + node + "/qemu/" + strconv.Itoa(vmid) + "/config"
}
