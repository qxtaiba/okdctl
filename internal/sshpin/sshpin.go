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

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/system"
)

// Verify runs ssh-keyscan against host and compares SHA256 fingerprints
// against expected. On match it writes a temp known_hosts file (caller
// must remove it) and returns its path; an empty expected warns and
// returns ("", nil) unless requirePinned (then errors); a mismatch
// returns an error listing expected vs observed fingerprints.
func Verify(ctx context.Context, host, expected string, requirePinned bool, log *slog.Logger) (string, error) {
	// ssh-keyscan runs without -H so hostnames appear in plain, deterministic
	// form (no random salt).
	out, err := executor.OutputCaptured(ctx, "ssh-keyscan", "-T", "5", host)
	if err != nil {
		return "", fmt.Errorf("ssh-keyscan %s: %w", host, err)
	}
	return parseAndMatch(string(out), host, expected, requirePinned, log)
}

func parseAndMatch(keyscanOut, host, expected string, requirePinned bool, log *slog.Logger) (string, error) {
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
		if requirePinned {
			return "", &errtypes.AuthError{Msg: "proxmox.require_pinned_fingerprint is set but ssh_host_fingerprint is empty"}
		}
		log.Warn("ssh: proxmox host fingerprint not pinned — set proxmox.ssh_host_fingerprint to one of the observed values",
			"host", host, "observed_fingerprints", strings.Join(observed, ", "))
		return "", nil
	}

	return "", &errtypes.AuthError{Msg: fmt.Sprintf("ssh host key mismatch for %s: expected %s, observed [%s]",
		host, expected, strings.Join(observed, ", "))}
}

func writeKnownHosts(matchedLine string) (string, error) {
	return system.WriteTempFile("okdctl-known-hosts-*", 0o600, func(f *os.File) error {
		_, err := fmt.Fprintln(f, matchedLine)
		return err
	})
}
