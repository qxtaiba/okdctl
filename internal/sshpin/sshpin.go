// Package sshpin validates a Proxmox SSH host-key fingerprint against an
// operator-configured SHA256 pin, writing a temp known_hosts file on match.
package sshpin

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"golang.org/x/crypto/ssh"

	"github.com/qxtaiba/okdctl/internal/system"
)

// Verify runs ssh-keyscan against host and compares each returned key's
// SHA256 fingerprint against expected.
//
// On match: writes a temp known_hosts file containing only the matched line
// and returns its path. Caller owns cleanup via defer os.Remove(path).
//
// When expected is empty: logs all observed fingerprints at WARN so the
// operator can set proxmox.ssh_host_fingerprint, then returns ("", nil).
// Callers pass the empty path to SSHRun/SSHRunArgv, preserving accept-new.
//
// When expected is non-empty and no key matches: returns an error containing
// both the expected and all observed fingerprints.
func Verify(ctx context.Context, host, expected string, log *slog.Logger) (string, error) {
	out, err := runKeyscan(ctx, host)
	if err != nil {
		return "", fmt.Errorf("ssh-keyscan %s: %w", host, err)
	}
	return parseAndMatch(out, host, expected, log)
}

// runKeyscan invokes ssh-keyscan without -H so hostnames appear in plain
// form, making output deterministic across invocations (no random salt).
func runKeyscan(ctx context.Context, host string) (string, error) {
	out, err := system.OutputCaptured(ctx, "ssh-keyscan", "-T", "5", host)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// parseAndMatch is the testable core: inspects keyscanOut line by line,
// parses each key, and compares its fingerprint against expected. Separated
// from Verify so tests can pass a fixed string without spawning a subprocess.
func parseAndMatch(keyscanOut, host, expected string, log *slog.Logger) (string, error) {
	var observed []string

	sc := bufio.NewScanner(strings.NewReader(keyscanOut))
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(line))
		if err != nil {
			continue
		}
		fp := ssh.FingerprintSHA256(key)
		if expected != "" && fp == expected {
			return writeKnownHosts(line)
		}
		observed = append(observed, fp)
	}

	if expected == "" {
		log.Warn("ssh: proxmox host fingerprint not pinned — set proxmox.ssh_host_fingerprint to one of the observed values",
			"host", host, "observed", strings.Join(observed, ", "))
		return "", nil
	}

	return "", fmt.Errorf("ssh host key mismatch for %s: expected %s, observed [%s]",
		host, expected, strings.Join(observed, ", "))
}

func writeKnownHosts(matchedLine string) (string, error) {
	return system.WriteTempFile("okdctl-known-hosts-*", 0o600, func(f *os.File) error {
		_, err := fmt.Fprintln(f, matchedLine)
		return err
	})
}
