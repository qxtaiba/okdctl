package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

func seedMarkerFile(t *testing.T, dir, relPath, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

const tfStateRelPath = "infrastructure/terraform/environments/production/terraform.tfstate"

func TestHasProjectMarker(t *testing.T) {
	cases := []struct {
		name    string
		seed    string // relative path to create; empty seeds nothing
		content string
		want    bool
	}{
		{"config file", "okdctl.yaml", "", true},
		{"env file", "okdctl.env", "", true},
		{"tfstate", tfStateRelPath, "{}", true},
		{"none", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.seed != "" {
				seedMarkerFile(t, dir, tc.seed, tc.content)
			}
			if got := hasProjectMarker(dir); got != tc.want {
				t.Errorf("hasProjectMarker = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLoadConfigMissingCarriesHint(t *testing.T) {
	t.Run("default path", func(t *testing.T) {
		t.Chdir(t.TempDir())

		_, err := loadConfig("okdctl.yaml")
		if !errors.Is(err, errtypes.ErrConfigMissing) {
			t.Fatalf("want ErrConfigMissing, got %v", err)
		}
		if got := exitCodeFor(err); got != 66 {
			t.Fatalf("exitCodeFor = %d, want 66", got)
		}
		d, ok := errtypes.Describe(err)
		if !ok {
			t.Fatalf("errtypes.Describe failed to classify %v", err)
		}
		if !strings.Contains(d.Hint, "okdctl deploy") {
			t.Fatalf("hint = %q, want it to contain %q", d.Hint, "okdctl deploy")
		}
	})

	t.Run("custom path", func(t *testing.T) {
		configFile := filepath.Join(t.TempDir(), "custom.yaml")

		_, err := loadConfig(configFile)
		if !errors.Is(err, errtypes.ErrConfigMissing) {
			t.Fatalf("want ErrConfigMissing, got %v", err)
		}
		d, ok := errtypes.Describe(err)
		if !ok {
			t.Fatalf("errtypes.Describe failed to classify %v", err)
		}
		if !strings.Contains(d.Hint, "--output-file") {
			t.Fatalf("hint = %q, want it to contain %q", d.Hint, "--output-file")
		}
	})
}
