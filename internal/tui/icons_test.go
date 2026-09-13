package tui

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIconsAreCheckAndCross(t *testing.T) {
	if IconSuccess != "✓" || IconError != "✗" {
		t.Fatalf("icons = %q %q", IconSuccess, IconError)
	}
}

func TestNoLiteralStatusGlyphsOutsideIcons(t *testing.T) {
	bad := []string{`"✓`, `✓ "`, `"✗`, `"●`, `"○`, `"✔`, `"✖`}
	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || filepath.Base(path) == "icons.go" {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, b := range bad {
			if strings.Contains(string(src), b) {
				t.Errorf("%s spells a status glyph literally (%s); use tui.Icon*", path, b)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
