package hostssh

import (
	"context"
	"fmt"
	"strconv"
)

// pveshRun executes a pvesh subcommand on the Proxmox host in argv mode so
// no shell string is assembled from user-controlled atoms. p.Node is
// validated once here; callers must not re-validate. extra is inserted
// between path and --output-format json (e.g. "-snapname", "foo" for a
// snapshot create call); callers must validate every extra atom themselves
// before passing it in, the same way p.Node is validated here — pveshRun
// performs no allowlist checks on extra.
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

// PveshRun is the exported entry point for callers outside package hostssh.
// resource is a node-relative path suffix (e.g. "qemu"); PveshRun composes
// /nodes/<node>/<resource> itself so the node atom can never bypass
// pveshRun's validateProxmoxName chokepoint, and callers must not validate
// p.Node themselves. Returns the raw JSON stdout on success.
func PveshRun(ctx context.Context, p *RemoteISOParams, subcommand, resource string, extra ...string) (string, error) {
	return pveshRun(ctx, p, subcommand, "/nodes/"+p.Node+"/"+resource, extra...)
}

func pveshQEMUPath(node string) string {
	return "/nodes/" + node + "/qemu"
}

func pveshConfigPath(node string, vmid int) string {
	return "/nodes/" + node + "/qemu/" + strconv.Itoa(vmid) + "/config"
}
