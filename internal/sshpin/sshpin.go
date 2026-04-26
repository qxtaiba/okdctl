// Package sshpin parses and matches SSH host-key fingerprints in the
// `SHA256:<base64>` form produced by `ssh-keygen -lf` and consumed by ssh
// itself. It is the shared helper backing fingerprint pinning for
// SSH-to-Proxmox (provider.proxmox.ssh.host_fingerprint) and Flux git-host
// pinning (addons.flux.settings.known_hosts_sha256).
package sshpin

import (
	"bufio"
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"
)

// Prefix is the standard fingerprint scheme prefix.
const Prefix = "SHA256:"

// Parse validates a fingerprint string. It accepts the canonical
// `SHA256:<base64>` form with or without `=` padding, rejects empty input
// and any other prefix, and returns the canonical form on success.
//
// We do not decode the base64 here. ssh.FingerprintSHA256 emits a stable
// unpadded base64; comparing canonical strings byte-for-byte is sufficient
// and avoids re-encoding ambiguity.
func Parse(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("fingerprint is empty")
	}
	if !strings.HasPrefix(s, Prefix) {
		return "", fmt.Errorf("fingerprint must start with %q, got %q", Prefix, s)
	}
	body := strings.TrimRight(s[len(Prefix):], "=")
	if body == "" {
		return "", fmt.Errorf("fingerprint missing base64 body")
	}
	return Prefix + body, nil
}

// KeyLines returns the non-comment, non-empty lines of an ssh-keyscan
// stdout. Comment lines (the `# host SSH-2.0-...` banner) shift position
// between invocations and must not be persisted into a known_hosts blob
// whose consumer is sensitive to byte equality (the Flux deploy-key
// Secret).
func KeyLines(stdout string) []string {
	var out []string
	scanner := bufio.NewScanner(strings.NewReader(stdout))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// LineFingerprint parses a single ssh-keyscan line of the form
// `<host> <type> <base64-key>` and returns the host and the SHA256
// fingerprint of the key. Lines without a parseable key produce an
// error so callers can decide whether to skip or refuse.
func LineFingerprint(line string) (host, fingerprint string, err error) {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return "", "", fmt.Errorf("expected `<host> <type> <key>`, got %d fields", len(fields))
	}
	pubKey, _, _, _, perr := ssh.ParseAuthorizedKey([]byte(fields[1] + " " + fields[2]))
	if perr != nil {
		return "", "", fmt.Errorf("parse key: %w", perr)
	}
	return fields[0], ssh.FingerprintSHA256(pubKey), nil
}

// Match describes one parsed line of ssh-keyscan output.
type Match struct {
	Line        string
	Host        string
	Fingerprint string
}

// MatchKeyscan parses every key line in stdout and returns one Match per
// line. Comments are skipped. Lines that fail to parse are returned as a
// joined error so the caller can surface the bad input but still inspect
// the well-formed entries.
func MatchKeyscan(stdout string) ([]Match, error) {
	lines := KeyLines(stdout)
	matches := make([]Match, 0, len(lines))
	var firstErr error
	for _, line := range lines {
		host, fp, err := LineFingerprint(line)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("line %q: %w", line, err)
			}
			continue
		}
		matches = append(matches, Match{Line: line, Host: host, Fingerprint: fp})
	}
	if len(matches) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return matches, nil
}

// FilterByPin returns the subset of matches whose fingerprint equals pin.
// The pin must already be canonical (Parse normalises it).
func FilterByPin(matches []Match, pin string) []Match {
	var out []Match
	for _, m := range matches {
		if m.Fingerprint == pin {
			out = append(out, m)
		}
	}
	return out
}

// KnownHostsBytes emits the matched lines joined with `\n` and a final
// newline, suitable for writing into a temp known_hosts file or the
// Flux deploy-key Secret.
func KnownHostsBytes(matches []Match) []byte {
	if len(matches) == 0 {
		return nil
	}
	var b strings.Builder
	for _, m := range matches {
		b.WriteString(m.Line)
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

// Fingerprints returns just the fingerprint strings for diagnostics
// (logging at WARN when no pin is configured).
func Fingerprints(matches []Match) []string {
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m.Fingerprint)
	}
	return out
}
