package sshpin

import (
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

const (
	// Fixed ed25519 public key (well-formed, generated for this test
	// fixture). The matching SHA256 fingerprint is computed once below.
	fixturePubKeyType = "ssh-ed25519"
	fixturePubKeyB64  = "AAAAC3NzaC1lZDI1NTE5AAAAIB7G3kgKE/2/wQE9hYS0RPAtbz/14ot8U30E1FK6apJk"
	fixtureHostA      = "192.168.1.10"
	fixtureHostB      = "[2001:db8::1]:22"
)

func mustComputeFingerprint(t *testing.T) string {
	t.Helper()
	pub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(fixturePubKeyType + " " + fixturePubKeyB64))
	if err != nil {
		t.Fatalf("parse fixture key: %v", err)
	}
	return ssh.FingerprintSHA256(pub)
}

func TestParse(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"happy unpadded", "SHA256:abc123def", "SHA256:abc123def", false},
		{"strips padding", "SHA256:abc123def==", "SHA256:abc123def", false},
		{"trims whitespace", "  SHA256:abc123def  ", "SHA256:abc123def", false},
		{"empty", "", "", true},
		{"wrong prefix", "MD5:aa:bb:cc", "", true},
		{"missing body", "SHA256:", "", true},
		{"plain base64 no prefix", "abc123def", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Parse(%q) err=%v wantErr=%v", tc.in, err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("Parse(%q)=%q want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestKeyLines_FiltersBannerAndBlanks(t *testing.T) {
	stdout := "# 192.168.1.10:22 SSH-2.0-OpenSSH_9.6\n" +
		"\n" +
		"192.168.1.10 ssh-ed25519 " + fixturePubKeyB64 + "\n" +
		"  \n" +
		"# 192.168.1.10:22 SSH-2.0-OpenSSH_9.6\n" +
		"192.168.1.10 ecdsa-sha2-nistp256 ABCD\n"
	lines := KeyLines(stdout)
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %#v", len(lines), lines)
	}
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "#") || strings.TrimSpace(l) == "" {
			t.Fatalf("KeyLines did not filter banner/blank: %q", l)
		}
	}
}

func TestLineFingerprint_RoundTrip(t *testing.T) {
	want := mustComputeFingerprint(t)
	line := fixtureHostA + " " + fixturePubKeyType + " " + fixturePubKeyB64
	host, fp, err := LineFingerprint(line)
	if err != nil {
		t.Fatalf("LineFingerprint: %v", err)
	}
	if host != fixtureHostA {
		t.Fatalf("host=%q want %q", host, fixtureHostA)
	}
	if fp != want {
		t.Fatalf("fp=%q want %q", fp, want)
	}
}

func TestLineFingerprint_Malformed(t *testing.T) {
	cases := []string{
		"",
		"only-host",
		"host type",
		"host bad-type AAAA",
	}
	for _, line := range cases {
		t.Run(line, func(t *testing.T) {
			if _, _, err := LineFingerprint(line); err == nil {
				t.Fatalf("LineFingerprint(%q) expected error", line)
			}
		})
	}
}

func TestMatchKeyscan_HappyPath(t *testing.T) {
	stdout := "# 192.168.1.10:22 SSH-2.0-OpenSSH_9.6\n" +
		fixtureHostA + " " + fixturePubKeyType + " " + fixturePubKeyB64 + "\n" +
		fixtureHostB + " " + fixturePubKeyType + " " + fixturePubKeyB64 + "\n"
	matches, err := MatchKeyscan(stdout)
	if err != nil {
		t.Fatalf("MatchKeyscan: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("got %d matches, want 2: %#v", len(matches), matches)
	}
	want := mustComputeFingerprint(t)
	for _, m := range matches {
		if m.Fingerprint != want {
			t.Fatalf("fingerprint=%q want %q", m.Fingerprint, want)
		}
	}
}

func TestMatchKeyscan_OrderInsensitive(t *testing.T) {
	pinned := mustComputeFingerprint(t)
	first := "# banner\n" +
		fixtureHostA + " " + fixturePubKeyType + " " + fixturePubKeyB64 + "\n"
	second := fixtureHostA + " " + fixturePubKeyType + " " + fixturePubKeyB64 + "\n" +
		"# banner-after\n"
	a, err := MatchKeyscan(first)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	b, err := MatchKeyscan(second)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if len(a) != 1 || len(b) != 1 {
		t.Fatalf("expected one match each, got %d / %d", len(a), len(b))
	}
	if a[0].Fingerprint != pinned || b[0].Fingerprint != pinned {
		t.Fatalf("fingerprints diverged across runs: %q vs %q", a[0].Fingerprint, b[0].Fingerprint)
	}
}

func TestFilterByPin(t *testing.T) {
	pinned := mustComputeFingerprint(t)
	matches := []Match{
		{Line: "a", Host: "h1", Fingerprint: pinned},
		{Line: "b", Host: "h2", Fingerprint: "SHA256:bogus"},
		{Line: "c", Host: "h3", Fingerprint: pinned},
	}
	got := FilterByPin(matches, pinned)
	if len(got) != 2 {
		t.Fatalf("FilterByPin matched %d, want 2", len(got))
	}
	if FilterByPin(matches, "SHA256:nothing") != nil {
		t.Fatalf("expected nil for no-match pin")
	}
}

func TestKnownHostsBytes(t *testing.T) {
	matches := []Match{
		{Line: "host1 ssh-ed25519 KEY1"},
		{Line: "host2 ssh-ed25519 KEY2"},
	}
	got := string(KnownHostsBytes(matches))
	want := "host1 ssh-ed25519 KEY1\nhost2 ssh-ed25519 KEY2\n"
	if got != want {
		t.Fatalf("KnownHostsBytes=%q want %q", got, want)
	}
	if KnownHostsBytes(nil) != nil {
		t.Fatalf("KnownHostsBytes(nil) should be nil")
	}
}

func TestKnownHostsBytes_NoComments(t *testing.T) {
	stdout := "# banner\nhost ssh-ed25519 " + fixturePubKeyB64 + "\n"
	matches, err := MatchKeyscan(stdout)
	if err != nil {
		t.Fatalf("MatchKeyscan: %v", err)
	}
	got := string(KnownHostsBytes(matches))
	if strings.Contains(got, "#") {
		t.Fatalf("KnownHostsBytes leaked comment: %q", got)
	}
}
