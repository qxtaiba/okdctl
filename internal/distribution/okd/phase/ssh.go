package phase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"strings"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
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

// VerifyProxmoxHostKey uses ssh-keyscan to fetch host's public keys, hashes
// them with SHA256, and compares against expected (a colon-stripped lowercase
// hex fingerprint or prefixed "SHA256:..." form). Empty expected logs the
// observed fingerprint at WARN so the operator can pin it on the next run.
//
// On mismatch with non-empty expected, returns a ClusterError to abort the
// deploy. Non-linux hosts and missing ssh-keyscan fall through with a WARN.
func VerifyProxmoxHostKey(ctx context.Context, exec *executor.Executor, host, expected string, log *slog.Logger) error {
	log = logutil.OrNop(log)
	result, err := exec.Run(ctx, "ssh-keyscan", "-H", "-T", "5", host)
	if err != nil {
		log.Warn("ssh: host-key-scan failed — proceeding without fingerprint verification",
			"host", host, "err", err)
		return nil
	}
	if result.ExitCode != 0 || strings.TrimSpace(result.Stdout) == "" {
		log.Warn("ssh: host-key-scan produced no output — proceeding without fingerprint verification",
			"host", host, "exit", result.ExitCode)
		return nil
	}
	sum := sha256.Sum256([]byte(result.Stdout))
	observed := hex.EncodeToString(sum[:])

	if expected == "" {
		log.Warn("ssh: no host-key fingerprint pinned — TOFU in effect",
			"host", host, "observed_sha256", observed,
			"hint", "set provider.proxmox.ssh_host_fingerprint to pin this key")
		return nil
	}

	if !sshFingerprintMatches(expected, observed) {
		return &errtypes.ClusterError{
			Msg: fmt.Sprintf("ssh host-key fingerprint mismatch for %s (expected %s, observed %s)",
				host, expected, observed),
		}
	}
	log.Info("ssh: host-key fingerprint verified", "host", host)
	return nil
}

func sshFingerprintMatches(expected, observed string) bool {
	return normalizeFingerprint(expected) == normalizeFingerprint(observed)
}

func normalizeFingerprint(s string) string {
	s = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "sha256:")
	return strings.ReplaceAll(s, ":", "")
}

// SSHRun runs a single command on root@host over SSH, reusing the shared
// flag set (-o StrictHostKeyChecking=accept-new -o BatchMode=yes).
// Non-zero exit codes do not produce an error; only transport failures do.
//
// Callers should invoke VerifyProxmoxHostKey once per deploy before the
// first SSHRun to honour provider.proxmox.ssh_host_fingerprint; this
// function itself preserves legacy accept-new TOFU.
func SSHRun(ctx context.Context, exec *executor.Executor, host, cmd string) (*executor.Result, error) {
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
