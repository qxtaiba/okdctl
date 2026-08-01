package hostssh

import (
	"context"
	"fmt"
	"strconv"

	"github.com/qxtaiba/okdctl/internal/executor"
)

// pveshRun executes a pvesh subcommand on the Proxmox host in argv mode so
// no shell string is assembled from user-controlled atoms. p.Node is
// validated once here; callers must not re-validate. extra is inserted
// between path and --output-format json (e.g. "-snapname", "foo" for a
// snapshot create call); callers must validate every extra atom themselves
// before passing it in, the same way p.Node is validated here — pveshRun
// performs no allowlist checks on extra.
//
// A non-zero pvesh exit is tolerated: the read-path callers parse whatever
// landed on stdout. Poll paths that must fail loudly on a rejected read use
// pveshRunChecked instead.
func pveshRun(ctx context.Context, p *RemoteISOParams, subcommand, path string, extra ...string) (string, error) {
	return pveshRunImpl(ctx, p, false, subcommand, path, extra...)
}

// pveshRunChecked is pveshRun that additionally fails when pvesh exits
// non-zero, routing the failure through executor.NewExitError so remote
// stderr is scrubbed and truncated. The poll in pveshWaitTask uses it so a
// permanently failing status read (expired UPID, wrong node) surfaces
// pvesh's actual stderr instead of a downstream JSON-parse artifact, and
// does not burn the whole timeout on a read that will never succeed.
func pveshRunChecked(ctx context.Context, p *RemoteISOParams, subcommand, path string, extra ...string) (string, error) {
	return pveshRunImpl(ctx, p, true, subcommand, path, extra...)
}

func pveshRunImpl(ctx context.Context, p *RemoteISOParams, checkExit bool, subcommand, path string, extra ...string) (string, error) {
	if err := validateProxmoxName(p.Node); err != nil {
		return "", fmt.Errorf("proxmox node %q invalid: %w", p.Node, err)
	}
	argv := append([]string{"pvesh", subcommand, path}, extra...)
	argv = append(argv, "--output-format", "json")
	result, err := SSHRunArgvOutput(ctx, p.Exec, p.Host, p.KnownHostsPath, argv...)
	if err != nil {
		return "", err
	}
	if checkExit && result.ExitCode != 0 {
		return "", fmt.Errorf("pvesh %s %s: %w", subcommand, path,
			executor.NewExitError(ctx, "pvesh "+subcommand, result.ExitCode, result.Stderr))
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
