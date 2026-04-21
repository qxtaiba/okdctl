package credentials

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

func TestEnvFilePath(t *testing.T) {
	cases := map[string]string{
		"okdctl.yaml":          "okdctl.env",
		"okdctl.yml":           "okdctl.env",
		"configs/cluster.yaml": "configs/cluster.env",
		"noext":                "noext.env",
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

func TestLoadEnvFile_PermRefusal(t *testing.T) {
	tests := []struct {
		name string
		perm os.FileMode
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
			if err := os.WriteFile(path, []byte("PROXMOX_VE_API_TOKEN=tok\n"), tc.perm); err != nil {
				t.Fatal(err)
			}
			// Explicit chmod in case umask narrowed the create mode.
			if err := os.Chmod(path, tc.perm); err != nil {
				t.Fatal(err)
			}

			// Call loadEnvFileOnce directly — LoadEnvFile's sync.Once would
			// short-circuit subsequent calls.
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
		// A file inside an unreadable dir: stat returns "permission denied",
		// not IsNotExist. Must wrap as ConfigError, not ignored.
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
