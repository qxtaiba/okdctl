package cli

import (
	"os"
	"path/filepath"
	"testing"
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
