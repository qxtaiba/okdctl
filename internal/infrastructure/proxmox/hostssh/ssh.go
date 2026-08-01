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

// sshRun runs a single command on root@host over SSH. It is unexported: the
// only sanctioned string-mode (sh -c) call site is RemoveFCOSISOFromProxmox
// in this package, so the shell-injection policy is compiler-enforced rather
// than prose-enforced. New cross-package SSH operations use SSHRunArgv.
//
// When knownHostsPath is non-empty the connection enforces strict host-key
// checking against that file. When empty, accept-new TOFU applies —
// preserving current behaviour. Non-zero exit codes do not produce an error;
// only transport failures do.
func sshRun(ctx context.Context, exec *executor.Executor, host, knownHostsPath, cmd string) (*executor.Result, error) {
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
// the shell. Callers MUST still validate every atom semantically before
// calling (pveshRun is the canonical example); as a fail-closed backstop,
// any atom containing a character outside [A-Za-z0-9@%+=:,./_-] is
// rejected here before ssh runs, so an unvalidated future caller cannot
// reintroduce remote injection.
//
// When knownHostsPath is non-empty the connection enforces strict host-key
// checking. When empty, accept-new TOFU applies.
func SSHRunArgv(ctx context.Context, exec *executor.Executor, host, knownHostsPath string, argv ...string) (*executor.Result, error) {
	if err := validateArgvAtoms(argv); err != nil {
		return nil, fmt.Errorf("ssh %s: %w", host, err)
	}
	args := sshBaseArgs(host, knownHostsPath)
	args = append(args, argv...)
	result, err := exec.Run(ctx, "ssh", args...)
	if err != nil {
		return result, fmt.Errorf("ssh %s: %w", host, err)
	}
	return result, nil
}

// sshRunOutput is sshRun with full stdout capture (Executor.RunOutput)
// instead of the ring-truncated tail, for callers that parse stdout as
// JSON or another format sensitive to truncation. Same non-zero-exit and
// shell-injection semantics as sshRun; likewise unexported.
func sshRunOutput(ctx context.Context, exec *executor.Executor, host, knownHostsPath, cmd string) (*executor.Result, error) {
	args := sshBaseArgs(host, knownHostsPath)
	args = append(args, cmd)
	result, err := exec.RunOutput(ctx, 0, "ssh", args...)
	if err != nil {
		return result, fmt.Errorf("ssh %s: %w", host, err)
	}
	return result, nil
}

// SSHRunArgvOutput is SSHRunArgv with full stdout capture (Executor.RunOutput)
// instead of the ring-truncated tail, for callers that parse stdout as JSON.
// Same non-zero-exit, argv-mode, and shell-safe-atom semantics as SSHRunArgv.
func SSHRunArgvOutput(ctx context.Context, exec *executor.Executor, host, knownHostsPath string, argv ...string) (*executor.Result, error) {
	if err := validateArgvAtoms(argv); err != nil {
		return nil, fmt.Errorf("ssh %s: %w", host, err)
	}
	args := sshBaseArgs(host, knownHostsPath)
	args = append(args, argv...)
	result, err := exec.RunOutput(ctx, 0, "ssh", args...)
	if err != nil {
		return result, fmt.Errorf("ssh %s: %w", host, err)
	}
	return result, nil
}

// validateArgvAtoms fails closed on any atom the remote login shell could
// reinterpret after ssh's space-join: only alphanumerics plus @%+=:,./_-
// (the shlex-safe set) are allowed, and empty atoms are rejected because
// they vanish in the join. Errors name the atom index and offending rune
// but never the atom itself, in case a future caller passes
// credential-bearing material.
func validateArgvAtoms(argv []string) error {
	for i, atom := range argv {
		if atom == "" {
			return fmt.Errorf("argv atom %d is empty and would vanish in ssh's space-join", i)
		}
		for _, r := range atom {
			if !argvAtomSafeRune(r) {
				return fmt.Errorf("argv atom %d contains shell-unsafe character %q", i, r)
			}
		}
	}
	return nil
}

func argvAtomSafeRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	}
	switch r {
	case '@', '%', '+', '=', ':', ',', '.', '/', '_', '-':
		return true
	}
	return false
}

// sshBaseArgs builds the ssh option flags and remote user@host token.
// Strict mode is used when knownHostsPath is set; accept-new otherwise.
// ConnectTimeout bounds connection establishment so a blackholed Proxmox
// host fails in seconds rather than stalling for the kernel TCP timeout on
// every one-shot pvesh/iso-cleanup call; ctx cancel remains the interactive
// escape hatch for the connected phase.
func sshBaseArgs(host, knownHostsPath string) []string {
	if knownHostsPath != "" {
		return []string{
			"-o", "UserKnownHostsFile=" + knownHostsPath,
			"-o", "StrictHostKeyChecking=yes",
			"-o", "BatchMode=yes",
			"-o", "ConnectTimeout=10",
			"root@" + host,
		}
	}
	return []string{
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=10",
		"root@" + host,
	}
}
