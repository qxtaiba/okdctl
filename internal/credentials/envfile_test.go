package credentials

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			creds := &ProxmoxCredentials{APIToken: []byte("test-secret")}
			var retained []byte
			want := errors.New("write failure")
			err := writeEnvFileBody(creds, func(data []byte) error {
				retained = data
				if !bytes.Contains(data, creds.APIToken) {
					t.Fatal("missing token")
				}
				if fail {
					return want
				}
				return nil
			})
			if (fail && !errors.Is(err, want)) || (!fail && err != nil) {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(retained) == 0 || !bytes.Equal(retained, make([]byte, len(retained))) {
				t.Fatal("write buffer not cleared")
			}
			if string(creds.APIToken) != "test-secret" {
				t.Fatal("caller token mutated")
			}
		})
	}
}

func TestWriteEnvFileRejectsLossyValues(t *testing.T) {
	for _, value := range []string{"secret\nPROXMOX_VE_INSECURE=true", "secret\rvalue", "secret\x00value", " secret", "secret ", "\"secret\"", "'secret'", strings.Repeat("s", 65536)} {
		t.Run(value[:min(len(value), 20)], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "credentials.env")
			mustWriteEnv(t, path, "original", 0o600)
			err := WriteEnvFile(path, &ProxmoxCredentials{APIToken: []byte(value)})
			if err == nil {
				t.Fatal("accepted unrepresentable value")
			}
			if strings.Contains(err.Error(), value) {
				t.Fatal("error exposes credential")
			}
			body, readErr := os.ReadFile(path)
			if readErr != nil || string(body) != "original" {
				t.Fatal("destination changed")
			}
		})
	}
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

func TestWriteEnvFile_TokenAndFlagOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "okdctl.env")
	creds := &ProxmoxCredentials{Endpoint: "https://pve:8006", APIToken: []byte("root@pam!okdctl=tok-abc"), Insecure: true}

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
	for _, line := range []string{"PROXMOX_VE_API_TOKEN=root@pam!okdctl=tok-abc", "PROXMOX_VE_INSECURE=true"} {
		if !strings.Contains(string(body), line) {
			t.Errorf("written .env missing %q; got:\n%s", line, body)
		}
	}
	if strings.Contains(string(body), "pve:8006") || strings.Contains(string(body), "PROXMOX_VE_ENDPOINT") {
		t.Errorf(".env carries the endpoint, which belongs in okdctl.yaml:\n%s", body)
	}

	verifying := filepath.Join(t.TempDir(), "okdctl.env")
	if err := WriteEnvFile(verifying, &ProxmoxCredentials{APIToken: []byte("tok")}); err != nil {
		t.Fatal(err)
	}
	body, _ = os.ReadFile(verifying)
	if strings.Contains(string(body), "PROXMOX_VE_INSECURE") {
		t.Errorf(".env sets PROXMOX_VE_INSECURE for a verifying connection:\n%s", body)
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
	err := WriteEnvFile(link, &ProxmoxCredentials{APIToken: []byte("tok")})
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
			path := filepath.Join(t.TempDir(), "perm.env")
			mustWriteEnv(t, path, "PROXMOX_VE_API_TOKEN=tok\n", tc.perm)
			if err := os.Chmod(path, tc.perm); err != nil {
				t.Fatal(err)
			}

			err := loadEnvFileOnce(path)
			var authErr *errtypes.AuthError
			if gotAuth := errors.As(err, &authErr); gotAuth != tc.wantAuthErr {
				t.Errorf("err = %v, wantAuthErr = %v", err, tc.wantAuthErr)
			}
		})
	}

	t.Run("missing file is not an error", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "does-not-exist.env")
		if err := loadEnvFileOnce(path); err != nil {
			t.Errorf("missing file should be nil error; got %v", err)
		}
	})

	t.Run("stat error on unreadable parent surfaces as ConfigError", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root bypasses perm bits")
		}
		inner := filepath.Join(t.TempDir(), "locked")
		if err := os.Mkdir(inner, 0o000); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(inner, 0o700) //nolint:errcheck // best-effort cleanup

		err := loadEnvFileOnce(filepath.Join(inner, "x.env"))
		if err == nil {
			t.Skip("filesystem allowed read despite 0o000")
		}
		var cfgErr *errtypes.ConfigError
		if !errors.As(err, &cfgErr) {
			t.Errorf("err = %v; want *errtypes.ConfigError", err)
		}
	})
}

// resetLoadOnceAfter clears the package-level LoadEnvFile guard once t ends,
// so a repeated in-process run starts from an unloaded state.
func resetLoadOnceAfter(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		loadOnce = sync.Once{}
		loadErr = nil
		loadedPath = ""
	})
}

func TestLoadEnvFile_SecondCallDifferentPath(t *testing.T) {
	resetLoadOnceAfter(t)
	dir := t.TempDir()
	first := filepath.Join(dir, "first.env")
	mustWriteEnv(t, first, "PROXMOX_VE_INSECURE=true\n", 0o600)
	if err := LoadEnvFile(first); err != nil {
		t.Fatalf("first LoadEnvFile: %v", err)
	}

	err := LoadEnvFile(filepath.Join(dir, "second.env"))
	if !errors.Is(err, errEnvFileAlreadyLoaded) {
		t.Errorf("err = %v; want errors.Is(errEnvFileAlreadyLoaded)", err)
	}
}

func TestLoadEnvFile_RejectsUnknownKeys(t *testing.T) {
	for _, stray := range []string{"TF_LOG", "PROXMOX_VE_USERNAME", "PROXMOX_VE_PASSWORD", "PROXMOX_VE_ENDPOINT"} {
		t.Run(stray, func(t *testing.T) {
			if _, present := os.LookupEnv(stray); present {
				t.Skipf("%s already set in the environment; cannot assert non-promotion", stray)
			}
			path := filepath.Join(t.TempDir(), "stray.env")
			mustWriteEnv(t, path, "PROXMOX_VE_API_TOKEN=tok\n"+stray+"=x\n", 0o600)

			err := loadEnvFileOnce(path)
			if !errors.Is(err, ErrEnvFileUnknownKey) {
				t.Fatalf("err = %v; want errors.Is(ErrEnvFileUnknownKey)", err)
			}
			if !strings.Contains(err.Error(), stray) {
				t.Errorf("error should name the rejected key; got %v", err)
			}
			if _, present := os.LookupEnv(stray); present {
				t.Errorf("rejected key %s was promoted into the environment despite the error", stray)
			}
		})
	}
}

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
