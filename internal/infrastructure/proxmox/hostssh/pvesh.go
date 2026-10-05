package hostssh

import (
	"context"
	"fmt"
	"strconv"
)

// pveshRun executes a pvesh subcommand in argv mode; p.Node is validated
// once here, but callers must validate every atom in extra themselves —
// pveshRun applies no allowlist there.
//
// A non-zero exit is tolerated: read-path callers parse whatever landed on
// stdout.
func pveshRun(ctx context.Context, p *RemoteISOParams, subcommand, path string, extra ...string) (string, error) {
	if err := validateProxmoxName(p.Node); err != nil {
		return "", fmt.Errorf("proxmox node %q invalid: %w", p.Node, err)
	}
	argv := append([]string{"pvesh", subcommand, path}, extra...)
	argv = append(argv, "--output-format", "json")
	result, err := SSHRunArgvOutput(ctx, p.Exec, p.Host, p.KnownHostsPath, argv...)
	if err != nil {
		return "", err
	}
	if result.Truncated {
		return "", fmt.Errorf("pvesh %s %s output truncated after %d bytes", subcommand, path, len(result.Stdout))
	}
	return result.Stdout, nil
}

// PveshRun is the exported entry point outside package hostssh; it composes
// /nodes/<node>/<resource> from resource (a node-relative suffix like
// "qemu") so the node atom can't bypass validateProxmoxName. Returns raw
// JSON stdout on success.
func PveshRun(ctx context.Context, p *RemoteISOParams, subcommand, resource string, extra ...string) (string, error) {
	return pveshRun(ctx, p, subcommand, "/nodes/"+p.Node+"/"+resource, extra...)
}

func pveshQEMUPath(node string) string {
	return "/nodes/" + node + "/qemu"
}

func pveshConfigPath(node string, vmid int) string {
	return "/nodes/" + node + "/qemu/" + strconv.Itoa(vmid) + "/config"
}
