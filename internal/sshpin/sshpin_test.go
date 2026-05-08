package sshpin

import (
	"os"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

const fixtureKeyscanLine = "pve.example ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl"

func fingerprintFromFixture(t *testing.T) string {
	t.Helper()
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(fixtureKeyscanLine))
	if err != nil {
		t.Fatalf("fixture key unparseable: %v", err)
	}
	return ssh.FingerprintSHA256(key)
}

func TestParseAndMatch_Match(t *testing.T) {
	fp := fingerprintFromFixture(t)
	path, err := parseAndMatch(fixtureKeyscanLine, "pve.example", fp, logutil.NopLogger)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if path == "" {
		t.Fatal("want non-empty known_hosts path on match; got empty string")
	}
	defer func() { _ = os.Remove(path) }()
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("cannot read known_hosts at %s: %v", path, readErr)
	}
	if !strings.Contains(string(content), "ssh-ed25519") {
		t.Errorf("known_hosts content %q missing key type", string(content))
	}
}

func TestParseAndMatch_Mismatch(t *testing.T) {
	fp := fingerprintFromFixture(t)
	wrong := "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	_, err := parseAndMatch(fixtureKeyscanLine, "pve.example", wrong, logutil.NopLogger)
	if err == nil {
		t.Fatal("expected error for fingerprint mismatch; got nil")
	}
	if !strings.Contains(err.Error(), wrong) {
		t.Errorf("error %q missing expected fingerprint %q", err.Error(), wrong)
	}
	if !strings.Contains(err.Error(), fp) {
		t.Errorf("error %q missing observed fingerprint %q", err.Error(), fp)
	}
}

func TestParseAndMatch_EmptyExpected(t *testing.T) {
	path, err := parseAndMatch(fixtureKeyscanLine, "pve.example", "", logutil.NopLogger)
	if err != nil {
		t.Fatalf("unexpected err for empty expected: %v", err)
	}
	if path != "" {
		t.Errorf("want empty path for empty expected; got %q", path)
	}
}
