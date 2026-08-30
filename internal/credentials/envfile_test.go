package credentials

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

func mustWriteEnv(t *testing.T, path, body string, perm os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), perm); err != nil {
		t.Fatal(err)
	}
}

func TestWriteEnvFile_BufferZeroedAfterWrite(t *testing.T) {
	const pw = "s3cret-pw"
	creds := &ProxmoxCredentials{
		Endpoint: "https://pve:8006",
		Username: "root@pam",
		Password: []byte(pw),
	}

	t.Run("WriteEnvFile writes password and pre-call slice is independent", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "zeroize.env")

		pre := buildEnvFileBody(creds)
		if !bytes.Contains(pre, []byte(pw)) {
			t.Fatalf("pre-call body missing password; got %q", pre)
		}

		if err := WriteEnvFile(path, creds); err != nil {
			t.Fatalf("WriteEnvFile: %v", err)
		}

		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if !bytes.Contains(body, []byte(pw)) {
			t.Errorf("written file missing password; got %q", body)
		}

		if !bytes.Contains(pre, []byte(pw)) {
			t.Errorf("pre slice was zeroed by WriteEnvFile; allocations must be independent")
		}
	})
}

func TestEnvFilePath(t *testing.T) {
	cases := map[string]string{
		"okdctl.yaml": "okdctl.env",
		"okdctl.yml":  "okdctl.env",
		"noext":       "noext.env",
	}
	for in, want := range cases {
		if got := EnvFilePath(in); got != want {
			t.Errorf("EnvFilePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWriteEnvFile_Perms(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "okdctl.env")

	creds := &ProxmoxCredentials{
		Endpoint: "https://pve:8006",
		Username: "root@pam",
		Password: []byte("hunter2"),
		Insecure: true,
	}
	if err := WriteEnvFile(path, creds); err != nil {
		t.Fatalf("WriteEnvFile failed: %v", err)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("perm = %#o, want 0o600", perm)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	want := []string{
		"PROXMOX_VE_ENDPOINT=https://pve:8006",
		"PROXMOX_VE_USERNAME=root@pam",
		"PROXMOX_VE_PASSWORD=hunter2",
		"PROXMOX_VE_INSECURE=true",
	}
	for _, line := range want {
		if !strings.Contains(string(body), line) {
			t.Errorf("written .env missing %q; got:\n%s", line, body)
		}
	}
}

func TestWriteEnvFile_APITokenOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.env")
	creds := &ProxmoxCredentials{
		Endpoint: "https://pve:8006",
		APIToken: []byte("tok-abc"),
	}
	if err := WriteEnvFile(path, creds); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(path)
	if strings.Contains(string(body), "PROXMOX_VE_USERNAME=") {
		t.Errorf(".env leaked empty username: %s", body)
	}
	if !strings.Contains(string(body), "PROXMOX_VE_API_TOKEN=tok-abc") {
		t.Errorf(".env missing api token; got:\n%s", body)
	}
}

func TestWriteEnvFile_SymlinkRefused(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses symlink restrictions")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "real.env")
	link := filepath.Join(dir, "okdctl.env")
	mustWriteEnv(t, target, "", 0o600)
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	creds := &ProxmoxCredentials{Endpoint: "https://pve:8006"}
	err := WriteEnvFile(link, creds)
	if err == nil {
		t.Fatal("WriteEnvFile should have refused symlink target")
	}
	var authErr *errtypes.AuthError
	if !errors.As(err, &authErr) {
		t.Errorf("err = %v; want *errtypes.AuthError", err)
	}
}

func TestLoadEnvFile_PermRefusal(t *testing.T) {
	tests := []struct {
		name        string
		perm        os.FileMode
		wantAuthErr bool
	}{
		{"0600 accepted", 0o600, false},
		{"0640 group-readable rejected", 0o640, true},
		{"0604 other-readable rejected", 0o604, true},
		{"0644 rejected", 0o644, true},
		{"0400 accepted", 0o400, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "perm.env")
			mustWriteEnv(t, path, "PROXMOX_VE_API_TOKEN=tok\n", tc.perm)
			// Explicit chmod in case umask narrowed the create mode.
			if err := os.Chmod(path, tc.perm); err != nil {
				t.Fatal(err)
			}

			// loadEnvFileOnce directly — LoadEnvFile's sync.Once would short-circuit.
			err := loadEnvFileOnce(path)
			var authErr *errtypes.AuthError
			gotAuth := errors.As(err, &authErr)
			if gotAuth != tc.wantAuthErr {
				t.Errorf("err = %v, wantAuthErr = %v", err, tc.wantAuthErr)
			}
		})
	}

	t.Run("missing file is not an error", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "does-not-exist.env")
		if err := loadEnvFileOnce(path); err != nil {
			t.Errorf("missing file should be nil error; got %v", err)
		}
	})

	t.Run("stat error on unreadable parent surfaces as ConfigError", func(t *testing.T) {
		// stat on an unreadable dir returns "permission denied", not IsNotExist.
		if os.Geteuid() == 0 {
			t.Skip("root bypasses perm bits")
		}
		dir := t.TempDir()
		inner := filepath.Join(dir, "locked")
		if err := os.Mkdir(inner, 0o000); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(inner, 0o700) //nolint:errcheck // best-effort cleanup
		path := filepath.Join(inner, "x.env")

		err := loadEnvFileOnce(path)
		if err == nil {
			t.Skip("filesystem allowed read despite 0o000 — likely macOS APFS or similar")
		}
		var cfgErr *errtypes.ConfigError
		if !errors.As(err, &cfgErr) {
			t.Errorf("err = %v; want *errtypes.ConfigError", err)
		}
	})
}

// TestLoadEnvFile_SecondCallDifferentPath is the only test in this
// package that calls the exported LoadEnvFile — its sync.Once is a
// process-global singleton, so every other test calls loadEnvFileOnce
// directly instead (see loadEnvFileOnce callers above).
func TestLoadEnvFile_SecondCallDifferentPath(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.env")
	mustWriteEnv(t, first, "PROXMOX_VE_INSECURE=true\n", 0o600)
	if err := LoadEnvFile(first); err != nil {
		t.Fatalf("first LoadEnvFile: %v", err)
	}

	second := filepath.Join(dir, "second.env")
	err := LoadEnvFile(second)
	if err == nil {
		t.Fatal("expected error for second LoadEnvFile call with a different path")
	}
	if !errors.Is(err, errEnvFileAlreadyLoaded) {
		t.Errorf("err = %v; want errors.Is(errEnvFileAlreadyLoaded)", err)
	}
}

// TestLoadEnvFile_RejectsUnknownKeys pins the allowlist: a key outside the
// PROXMOX_VE_* set must fail the whole load (naming the offender) and must not
// be promoted into the process environment, where it would reach every
// subprocess including terraform under the sudo re-exec.
func TestLoadEnvFile_RejectsUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stray.env")
	mustWriteEnv(t, path, "PROXMOX_VE_API_TOKEN=tok\nTF_LOG=DEBUG\n", 0o600)

	if _, present := os.LookupEnv("TF_LOG"); present {
		t.Skip("TF_LOG already set in the environment; cannot assert non-promotion")
	}

	err := loadEnvFileOnce(path)
	if !errors.Is(err, ErrEnvFileUnknownKey) {
		t.Fatalf("err = %v; want errors.Is(ErrEnvFileUnknownKey)", err)
	}
	if !strings.Contains(err.Error(), "TF_LOG") {
		t.Errorf("error should name the rejected key; got %v", err)
	}
	if _, present := os.LookupEnv("TF_LOG"); present {
		t.Errorf("rejected key TF_LOG was promoted into the environment despite the error")
	}
}

// TestLoadEnvFile_SymlinkRefused pins the O_NOFOLLOW gate: the permission
// decision and the read must bind to one inode, so a symlinked .env path is
// refused before its contents are read.
func TestLoadEnvFile_SymlinkRefused(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses O_NOFOLLOW semantics differently")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "real.env")
	link := filepath.Join(dir, "link.env")
	mustWriteEnv(t, target, "PROXMOX_VE_API_TOKEN=tok\n", 0o600)
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	err := loadEnvFileOnce(link)
	if err == nil {
		t.Fatal("loadEnvFileOnce should refuse a symlinked .env path")
	}
	var cfgErr *errtypes.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Errorf("err = %v; want *errtypes.ConfigError from the O_NOFOLLOW open", err)
	}
}
