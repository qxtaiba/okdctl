package phase

import (
	"os/user"
	"path/filepath"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
)

func TestResolveBinDir_Precedence(t *testing.T) {
	cases := []struct {
		name    string
		envVal  string
		cfgDir  string
		wantDir string
	}{
		{
			name:    "env wins over config and default",
			envVal:  "/opt/okdctl/bin",
			cfgDir:  "/config/bin",
			wantDir: "/opt/okdctl/bin",
		},
		{
			name:    "config wins over default when env unset",
			envVal:  "",
			cfgDir:  "/config/bin",
			wantDir: "/config/bin",
		},
		{
			name:    "default when env and config both absent",
			envVal:  "",
			cfgDir:  "",
			wantDir: DefaultBinDir,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OKDCTL_BIN_DIR", tc.envVal)
			var cfg *config.Config
			if tc.cfgDir != "" {
				cfg = &config.Config{}
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
	// relative and empty env values must be skipped; control falls to config
	// then to DefaultBinDir. Locked so a future ValidateBinDir relaxation is a
	// visible diff here before it reaches production.
	cases := []struct {
		name    string
		envVal  string
		cfgDir  string
		wantDir string
	}{
		{
			name:    "relative env falls through to config",
			envVal:  "relative/bin",
			cfgDir:  "/config/bin",
			wantDir: "/config/bin",
		},
		{
			name:    "relative env falls through to default when config absent",
			envVal:  "relative/bin",
			cfgDir:  "",
			wantDir: DefaultBinDir,
		},
		{
			name:    "nil cfg falls back to default",
			envVal:  "",
			cfgDir:  "",
			wantDir: DefaultBinDir,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OKDCTL_BIN_DIR", tc.envVal)
			var cfg *config.Config
			if tc.cfgDir != "" {
				cfg = &config.Config{}
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
	// Mirrors the SUDO_USER seam in system/fs_test.go TestExpandPath: ~/bin in
	// config must expand to the invoking user's home even under sudo.
	cur, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SUDO_USER", cur.Username)
	t.Setenv("OKDCTL_BIN_DIR", "")

	cfg := &config.Config{}
	cfg.Deployment.BinDir = "~/bin"

	got := ResolveBinDir(cfg)
	want := filepath.Join(cur.HomeDir, "bin")
	if got != want {
		t.Errorf("ResolveBinDir(~/bin) = %q; want %q", got, want)
	}
}

// TestResolveBinDir_DotDotTraversal locks current behavior: dot-dot sequences
// are collapsed by filepath.Clean but not rejected. A future change adding
// traversal rejection must update both the impl and this assertion.
func TestResolveBinDir_DotDotTraversal(t *testing.T) {
	t.Setenv("OKDCTL_BIN_DIR", "")

	cfg := &config.Config{}
	cfg.Deployment.BinDir = "/usr/local/bin/../../etc"

	got := ResolveBinDir(cfg)
	want := filepath.Clean("/usr/local/bin/../../etc")
	if got != want {
		t.Errorf("ResolveBinDir dot-dot = %q; want %q (traversal not rejected, only cleaned)", got, want)
	}
}

func TestDefaultBinDir_NotACriticalPath(t *testing.T) {
	// cleanup criticalPaths includes "/usr/local" but not "/usr/local/bin".
	// Cross-assert without importing cleanup (which imports phase, so the
	// reverse import would cycle).
	cleaned := filepath.Clean(DefaultBinDir)
	if cleaned == "/usr/local" {
		t.Errorf("DefaultBinDir cleans to /usr/local; cleanup refuseCriticalPath would reject it")
	}
	if cleaned != DefaultBinDir {
		t.Errorf("DefaultBinDir %q is not already clean; filepath.Clean = %q", DefaultBinDir, cleaned)
	}
}
