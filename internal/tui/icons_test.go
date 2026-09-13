package tui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
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
	bad := []rune{'✓', '✗', '✔', '✖', '●', '○'}
	fset := token.NewFileSet()
	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || filepath.Base(path) == "icons.go" {
			return err
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			for _, r := range bad {
				if strings.ContainsRune(lit.Value, r) {
					t.Errorf("%s:%d spells a status glyph literally (%q); use tui.Icon*",
						path, fset.Position(lit.Pos()).Line, r)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
