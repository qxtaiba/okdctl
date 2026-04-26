package phase

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"

	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/sshpin"
	"github.com/qxtaiba/okdctl/internal/system"
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

// SSHRun runs a single command on root@host over SSH. When fingerprint is
// the `SHA256:<base64>` form of an advertised host key, the function pins
// to exactly that key via a per-call temp known_hosts file and refuses if
// no advertised key matches. When fingerprint is empty it falls back to
// accept-new TOFU and, if log is non-nil, warns with the observed
// fingerprints so the operator can pin one. Non-zero exit codes do not
// produce an error; only transport / setup failures do.
func SSHRun(ctx context.Context, exec *executor.Executor, log *slog.Logger, host, fingerprint, cmd string) (*executor.Result, error) {
	if fingerprint == "" {
		if log != nil {
			if obs, err := ObserveSSHFingerprints(ctx, exec, host); err == nil && len(obs) > 0 {
				log.Warn("ssh: host key not pinned; set provider.proxmox.ssh.host_fingerprint to one of the observed values",
					"host", host, "observed", obs)
			}
		}
		result, err := exec.Run(ctx, "ssh",
			"-o", "StrictHostKeyChecking=accept-new",
			"-o", "BatchMode=yes",
			"root@"+host,
			cmd,
		)
		if err != nil {
			return result, fmt.Errorf("ssh %s: %w", host, err)
		}
		return result, nil
	}

	knownHostsPath, cleanup, err := PinnedKnownHostsFile(ctx, exec, host, fingerprint)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	result, err := exec.Run(ctx, "ssh",
		"-o", "UserKnownHostsFile="+knownHostsPath,
		"-o", "StrictHostKeyChecking=yes",
		"-o", "BatchMode=yes",
		"root@"+host,
		cmd,
	)
	if err != nil {
		return result, fmt.Errorf("ssh %s: %w", host, err)
	}
	return result, nil
}

// PinnedKnownHostsFile keyscans host, validates that fingerprint matches one
// of the advertised keys, and writes the matched key line(s) to a temp
// known_hosts file (mode 0o600). The returned cleanup removes the file;
// callers must defer it. fingerprint must be non-empty in `SHA256:<base64>`
// form. Used by SSHRun and the scp upload path so both share the
// keyscan + pin + temp-file flow.
func PinnedKnownHostsFile(ctx context.Context, exec *executor.Executor, host, fingerprint string) (path string, cleanup func(), err error) {
	noop := func() {}
	pin, err := sshpin.Parse(fingerprint)
	if err != nil {
		return "", noop, fmt.Errorf("ssh fingerprint for %s: %w", host, err)
	}
	matches, err := keyscan(ctx, exec, host)
	if err != nil {
		return "", noop, fmt.Errorf("ssh-keyscan %s: %w", host, err)
	}
	matched := sshpin.FilterByPin(matches, pin)
	if len(matched) == 0 {
		return "", noop, fmt.Errorf(
			"ssh: pinned fingerprint %s for %s does not match any advertised key (observed %v)",
			pin, host, sshpin.Fingerprints(matches))
	}
	path, err = system.WriteTempFile("okdctl-known-hosts-*", 0o600, func(f *os.File) error {
		_, werr := f.Write(sshpin.KnownHostsBytes(matched))
		return werr
	})
	if err != nil {
		return "", noop, fmt.Errorf("write known_hosts: %w", err)
	}
	return path, func() { _ = os.Remove(path) }, nil
}

func keyscan(ctx context.Context, exec *executor.Executor, host string) ([]sshpin.Match, error) {
	result, err := exec.Run(ctx, "ssh-keyscan", host)
	if err != nil {
		return nil, err
	}
	return sshpin.MatchKeyscan(result.Stdout)
}

// ObserveSSHFingerprints runs ssh-keyscan against host and returns the
// SHA256 fingerprints of every advertised key. Used for the unpinned WARN
// log so the operator can copy a value into provider.proxmox.ssh.host_fingerprint.
func ObserveSSHFingerprints(ctx context.Context, exec *executor.Executor, host string) ([]string, error) {
	matches, err := keyscan(ctx, exec, host)
	if err != nil {
		return nil, err
	}
	return sshpin.Fingerprints(matches), nil
}
