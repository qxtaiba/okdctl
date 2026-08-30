package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSeverityString(t *testing.T) {
	cases := []struct {
		sev  Severity
		want string
	}{
		{Pass, "ok"},
		{Warn, "warn"},
		{Fail, "fail"},
		{Severity(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.sev.String(); got != tc.want {
			t.Errorf("Severity(%d).String() = %q; want %q", tc.sev, got, tc.want)
		}
	}
}

func TestBinDirResolutionDemote(t *testing.T) {
	if got := (binDirResolution{}).demote(Pass); got != Pass {
		t.Errorf("demote(Pass) = %v; want Pass", got)
	}

	failed := binDirResolution{LoadFailed: true}
	if got := failed.demote(Pass); got != Warn {
		t.Errorf("demote(Pass) = %v; want Warn", got)
	}
	if got := failed.demote(Fail); got != Fail {
		t.Errorf("demote(Fail) = %v; want Fail", got)
	}
}

// writePullSecretConfig writes a pull-secret file and okdctl.yaml pointing at it.
func writePullSecretConfig(t *testing.T, psContent string) string {
	t.Helper()
	dir := t.TempDir()
	psPath := filepath.Join(dir, "pull-secret.json")
	if err := os.WriteFile(psPath, []byte(psContent), 0o600); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, "okdctl.yaml")
	cfgYAML := "schemaVersion: v2\nfiles:\n  pull_secret: " + psPath + "\n"
	if err := os.WriteFile(cfgPath, []byte(cfgYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	return cfgPath
}

func TestCheckPullSecret(t *testing.T) {
	t.Run("missing config warns", func(t *testing.T) {
		r := checkPullSecret(filepath.Join(t.TempDir(), "okdctl.yaml"))
		if r.Sev != Warn {
			t.Errorf("Sev = %v; want Warn (detail: %s)", r.Sev, r.Detail)
		}
	})

	t.Run("unset pull secret path fails", func(t *testing.T) {
		cfgPath := filepath.Join(t.TempDir(), "okdctl.yaml")
		if err := os.WriteFile(cfgPath, []byte("schemaVersion: v2\ncluster:\n  name: prod\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		r := checkPullSecret(cfgPath)
		if r.Sev != Fail || !strings.Contains(r.Detail, "files.pull_secret not set") {
			t.Errorf("Sev = %v, Detail = %q; want Fail with unset message", r.Sev, r.Detail)
		}
	})

	t.Run("valid pull secret passes", func(t *testing.T) {
		r := checkPullSecret(writePullSecretConfig(t, `{"auths":{"quay.io":{"auth":"x"}}}`))
		if r.Sev != Pass {
			t.Errorf("Sev = %v, Detail = %q; want Pass", r.Sev, r.Detail)
		}
	})

	t.Run("empty auths fails", func(t *testing.T) {
		r := checkPullSecret(writePullSecretConfig(t, `{"auths":{}}`))
		if r.Sev != Fail || !strings.Contains(r.Detail, "'auths' is empty") {
			t.Errorf("Sev = %v, Detail = %q; want Fail with empty-auths message", r.Sev, r.Detail)
		}
	})
}

func TestCheckSSHKey(t *testing.T) {
	t.Run("no keys warns", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		r := checkSSHKey(t.Context())
		if r.Sev != Warn || !strings.Contains(r.Detail, "no default ssh public key found") {
			t.Errorf("Sev = %v, Detail = %q; want Warn with no-key message", r.Sev, r.Detail)
		}
	})

	t.Run("key present passes", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		sshDir := filepath.Join(home, ".ssh")
		if err := os.MkdirAll(sshDir, 0o700); err != nil {
			t.Fatal(err)
		}
		keyPath := filepath.Join(sshDir, "id_ed25519.pub")
		if err := os.WriteFile(keyPath, []byte("ssh-ed25519 AAAA test"), 0o600); err != nil {
			t.Fatal(err)
		}
		r := checkSSHKey(t.Context())
		if r.Sev != Pass || r.Detail != keyPath {
			t.Errorf("Sev = %v, Detail = %q; want Pass with %q", r.Sev, r.Detail, keyPath)
		}
	})
}
