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

func TestLoadConfigFailureCarriesCause(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses the config permission gate")
	}
	cases := []struct {
		name string
		body string
		perm os.FileMode
		want []string
	}{
		{"unsupported schemaVersion", "schemaVersion: v1\n", 0o600, []string{`schemaVersion "v1"`, `expected "v2"`}},
		{"missing schemaVersion", "cluster:\n  name: demo\n", 0o600, []string{"missing required schemaVersion", `expected "v2"`}},
		{"unknown key", "schemaVersion: v2\nclutser:\n  name: demo\n", 0o600, []string{`unknown field "clutser"`}},
		{"several documents", "schemaVersion: v2\n---\ncluster:\n  name: demo\n", 0o600, []string{"more than one YAML document"}},
		{"group-writable file", "schemaVersion: v2\n", 0o660, []string{"insecure permissions 0660"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "okdctl.yaml")
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, tc.perm); err != nil {
				t.Fatal(err)
			}

			_, err := loadConfig(path)
			if err == nil {
				t.Fatal("loadConfig accepted the file")
			}
			msg := err.Error()
			if !strings.HasPrefix(msg, "config error: load configuration: ") {
				t.Errorf("error = %q; want the load configuration prefix followed by a cause", msg)
			}
			for _, want := range append(tc.want, path) {
				if !strings.Contains(msg, want) {
					t.Errorf("error = %q; want it to contain %q", msg, want)
				}
			}
			if got := exitCodeFor(err); got != 2 {
				t.Errorf("exitCodeFor = %d, want 2", got)
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
