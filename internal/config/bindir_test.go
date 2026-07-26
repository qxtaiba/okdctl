package config

import (
	"os/user"
	"path/filepath"
	"testing"
)

func TestResolveBinDir_Precedence(t *testing.T) {
	cases := []struct {
		name    string
		envVal  string
		cfgDir  string
		wantDir string
	}{
		{"env wins over config and default", "/opt/okdctl/bin", "/config/bin", "/opt/okdctl/bin"},
		{"config wins over default when env unset", "", "/config/bin", "/config/bin"},
		{"default when env and config both absent", "", "", DefaultBinDir},
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

func TestResolveBinDir_FallThroughOnInvalid(t *testing.T) {
	cases := []struct {
		name    string
		envVal  string
		cfgDir  string
		wantDir string
	}{
		{"relative env falls through to config", "relative/bin", "/config/bin", "/config/bin"},
		{"relative env falls through to default when config absent", "relative/bin", "", DefaultBinDir},
		{"nil cfg falls back to default", "", "", DefaultBinDir},
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

func TestBinDirOrDefault(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantDir string
	}{
		{"non-empty passes through", "/custom/bin", "/custom/bin"},
		{"empty falls back to default", "", DefaultBinDir},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := BinDirOrDefault(tc.in)
			if got != tc.wantDir {
				t.Errorf("BinDirOrDefault(%q) = %q; want %q", tc.in, got, tc.wantDir)
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

func TestResolveBinDir_DotDotTraversal(t *testing.T) {
	t.Run("config value with dot-dot falls back to default", func(t *testing.T) {
		t.Setenv("OKDCTL_BIN_DIR", "")
		cfg := &Config{}
		cfg.Deployment.BinDir = "/usr/local/bin/../../etc"
		if got := ResolveBinDir(cfg); got != DefaultBinDir {
			t.Errorf("ResolveBinDir dot-dot = %q; want %q (traversal rejected)", got, DefaultBinDir)
		}
	})

	t.Run("env value with dot-dot falls through to config", func(t *testing.T) {
		t.Setenv("OKDCTL_BIN_DIR", "/opt/../etc/bin")
		cfg := &Config{}
		cfg.Deployment.BinDir = "/config/bin"
		if got := ResolveBinDir(cfg); got != "/config/bin" {
			t.Errorf("ResolveBinDir = %q; want /config/bin", got)
		}
	})
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
