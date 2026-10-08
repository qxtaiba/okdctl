package hostssh

import (
	"context"
	"fmt"

	"github.com/qxtaiba/okdctl/internal/executor"
)

// DefaultProxmoxISODir is the ISO directory of a stock Proxmox VE `local`
// storage.
const DefaultProxmoxISODir = "/var/lib/vz/template/iso"

// PveshParams carries the connection parameters for pvesh queries against a
// Proxmox host. Host must be a bare hostname or IP (no port); an empty
// KnownHostsPath allows accept-new TOFU, otherwise strict host-key checking
// applies.
type PveshParams struct {
	Host           string
	Node           string
	Exec           *executor.Executor
	KnownHostsPath string
}

// validateProxmoxName is the defense-in-depth guard at the pveshRun
// boundary, catching a hand-edited YAML that bypasses
// config.ValidateOKDConfig.
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

// pveshRun executes a pvesh subcommand in argv mode; p.Node is validated
// once here, but callers must validate every atom in extra themselves —
// pveshRun applies no allowlist there.
//
// A non-zero exit is tolerated: read-path callers parse whatever landed on
// stdout.
func pveshRun(ctx context.Context, p *PveshParams, subcommand, path string, extra ...string) (string, error) {
	if err := validateProxmoxName(p.Node); err != nil {
		return "", fmt.Errorf("proxmox node %q invalid: %w", p.Node, err)
	}
	argv := append([]string{"pvesh", subcommand, path}, extra...)
	argv = append(argv, "--output-format", "json")
	result, err := sshRunArgvOutput(ctx, p.Exec, p.Host, p.KnownHostsPath, argv...)
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
func PveshRun(ctx context.Context, p *PveshParams, subcommand, resource string, extra ...string) (string, error) {
	return pveshRun(ctx, p, subcommand, "/nodes/"+p.Node+"/"+resource, extra...)
}
