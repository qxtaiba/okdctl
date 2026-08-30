// Package hostssh runs commands on the Proxmox host as root over SSH:
// command execution, pvesh queries, and CoreOS ISO cleanup. Policy: every
// SSH op uses SSHRunArgv except the single sanctioned sh -c call in
// RemoveFCOSISOFromProxmox.
package hostssh

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/qxtaiba/okdctl/internal/executor"
)

// ProxmoxBareHost strips any port suffix or URL scheme so the result can be
// passed directly to ssh (config may store "host:8006" or "https://host").
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

// sshRun runs a single command on root@host over SSH; it's unexported so
// RemoveFCOSISOFromProxmox stays the only sh -c call site, making the
// shell-injection policy compiler-enforced. New cross-package operations
// use SSHRunArgv.
//
// knownHostsPath non-empty enforces strict host-key checking; empty applies
// accept-new TOFU. Only transport failures error — non-zero exit codes don't.
func sshRun(ctx context.Context, exec *executor.Executor, host, knownHostsPath, cmd string) (*executor.Result, error) {
	args := sshBaseArgs(host, knownHostsPath)
	args = append(args, cmd)
	result, err := exec.Run(ctx, "ssh", args...)
	if err != nil {
		return result, fmt.Errorf("ssh %s: %w", host, err)
	}
	return result, nil
}

// SSHRunArgv passes each argv element to ssh as a separate argument; ssh(1)
// still space-joins them into one command string for the remote shell, so
// argv mode does NOT bypass it. Callers MUST validate every atom themselves
// (pveshRun is the canonical example) — as a fail-closed backstop, any atom
// outside [A-Za-z0-9@%+=:,./_-] is rejected here first.
//
// knownHostsPath non-empty enforces strict host-key checking; empty applies accept-new TOFU.
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
// instead of the ring-truncated tail, for callers parsing stdout as JSON;
// same semantics as sshRun.
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
// instead of the ring-truncated tail, for JSON-parsing callers; same
// semantics as SSHRunArgv.
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

// validateArgvAtoms fails closed on any atom outside the shlex-safe set
// [A-Za-z0-9@%+=:,./_-]; empty atoms are rejected too since they vanish in
// ssh's space-join. Errors name the atom's index and offending rune but
// never the atom itself, in case it carries credential material.
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

// sshBaseArgs builds the ssh option flags and user@host token — strict mode
// when knownHostsPath is set, accept-new otherwise. ConnectTimeout bounds
// connection setup so a blackholed host fails in seconds instead of the
// kernel TCP timeout; ctx cancel remains the escape hatch once connected.
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
