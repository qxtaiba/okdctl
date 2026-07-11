// Package hostssh runs commands on the Proxmox host as root over SSH:
// generic command execution, pvesh queries, and CoreOS ISO cleanup.
// Policy of record: all SSH operations use SSHRunArgv (argv-mode) except
// the single sanctioned sh -c call site in RemoveFCOSISOFromProxmox.
package hostssh

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/qxtaiba/okdctl/internal/executor"
)

// ProxmoxBareHost strips any port suffix or URL scheme from the host so the
// result can be passed directly to ssh. Proxmox hosts in config may appear as
// "host:8006" or "https://host".
func ProxmoxBareHost(host string) string {
	if idx := strings.Index(host, "://"); idx != -1 {
		host = host[idx+3:]
	}
	if strings.Contains(host, ":") {
		if h, _, err := net.SplitHostPort(host); err == nil {
			return h
		}
	}
	return host
}

// SSHRun runs a single command on root@host over SSH.
//
// When knownHostsPath is non-empty the connection enforces strict host-key
// checking against that file. When empty, accept-new TOFU applies —
// preserving current behaviour. Non-zero exit codes do not produce an error;
// only transport failures do.
func SSHRun(ctx context.Context, exec *executor.Executor, host, knownHostsPath, cmd string) (*executor.Result, error) {
	args := sshBaseArgs(host, knownHostsPath)
	args = append(args, cmd)
	result, err := exec.Run(ctx, "ssh", args...)
	if err != nil {
		return result, fmt.Errorf("ssh %s: %w", host, err)
	}
	return result, nil
}

// SSHRunArgv passes each argv element to ssh as a separate non-option
// argument. ssh(1) joins the trailing args with spaces and sends one
// command string to the remote login shell — argv mode does NOT bypass
// the shell. Callers MUST validate every atom for shell metacharacters
// before calling; pveshRun is the canonical example.
//
// When knownHostsPath is non-empty the connection enforces strict host-key
// checking. When empty, accept-new TOFU applies.
func SSHRunArgv(ctx context.Context, exec *executor.Executor, host, knownHostsPath string, argv ...string) (*executor.Result, error) {
	args := sshBaseArgs(host, knownHostsPath)
	args = append(args, argv...)
	result, err := exec.Run(ctx, "ssh", args...)
	if err != nil {
		return result, fmt.Errorf("ssh %s: %w", host, err)
	}
	return result, nil
}

// sshBaseArgs builds the ssh option flags and remote user@host token.
// Strict mode is used when knownHostsPath is set; accept-new otherwise.
func sshBaseArgs(host, knownHostsPath string) []string {
	if knownHostsPath != "" {
		return []string{
			"-o", "UserKnownHostsFile=" + knownHostsPath,
			"-o", "StrictHostKeyChecking=yes",
			"-o", "BatchMode=yes",
			"root@" + host,
		}
	}
	return []string{
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "BatchMode=yes",
		"root@" + host,
	}
}
