package config

import (
	"os/user"
	"path/filepath"
	"testing"
)

func TestResolveBinDir(t *testing.T) {
	cases := []struct {
		name    string
		envVal  string
		cfgDir  string
		wantDir string
	}{
		{"env wins over config and default", "/opt/okdctl/bin", "/config/bin", "/opt/okdctl/bin"},
		{"config wins over default when env unset", "", "/config/bin", "/config/bin"},
		{"nil cfg and unset env fall back to default", "", "", DefaultBinDir},
		{"relative env falls through to config", "relative/bin", "/config/bin", "/config/bin"},
		{"relative env falls through to default when config absent", "relative/bin", "", DefaultBinDir},
		{"config value with dot-dot falls back to default", "", "/usr/local/bin/../../etc", DefaultBinDir},
		{"env value with dot-dot falls through to config", "/opt/../etc/bin", "/config/bin", "/config/bin"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OKDCTL_BIN_DIR", tc.envVal)
			var cfg *Config
			if tc.cfgDir != "" {
				cfg = &Config{}
				cfg.Deployment.BinDir = tc.cfgDir
			}
			got := ResolveBinDir(cfg)
			if got != tc.wantDir {
				t.Errorf("ResolveBinDir = %q; want %q", got, tc.wantDir)
			}
		})
	}
}

func TestPreflightBinDir(t *testing.T) {
	cases := []struct {
		name    string
		envVal  string
		wantDir string
	}{
		{"absolute env wins", "/opt/okdctl/bin", "/opt/okdctl/bin"},
		{"empty env yields default", "", DefaultBinDir},
		{"relative env yields default", "relative/bin", DefaultBinDir},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OKDCTL_BIN_DIR", tc.envVal)
			got := PreflightBinDir()
			if got != tc.wantDir {
				t.Errorf("PreflightBinDir = %q; want %q", got, tc.wantDir)
			}
		})
	}
}

func TestResolveBinDir_TildeExpansion(t *testing.T) {
	cur, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SUDO_USER", cur.Username)
	t.Setenv("OKDCTL_BIN_DIR", "")

	cfg := &Config{}
	cfg.Deployment.BinDir = "~/bin"

	got := ResolveBinDir(cfg)
	want := filepath.Join(cur.HomeDir, "bin")
	if got != want {
		t.Errorf("ResolveBinDir(~/bin) = %q; want %q", got, want)
	}
}

func TestDefaultBinDir_NotACriticalPath(t *testing.T) {
	cleaned := filepath.Clean(DefaultBinDir)
	if cleaned == "/usr/local" {
		t.Errorf("DefaultBinDir cleans to /usr/local; cleanup refuseCriticalPath would reject it")
	}
	if cleaned != DefaultBinDir {
		t.Errorf("DefaultBinDir %q is not already clean; filepath.Clean = %q", DefaultBinDir, cleaned)
	}
}
