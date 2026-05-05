package httputil_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNewInsecureCallerPolicy fails if any file outside
// internal/distribution/okd/postinstall/ both imports httputil and calls
// httputil.NewInsecure. TLS-skip clients are only legitimate during the
// bootstrap window where no cluster CA is yet available; new callers must
// add themselves to allowedPrefixes after a security review.
func TestNewInsecureCallerPolicy(t *testing.T) {
	const importPath = "github.com/qxtaiba/okdctl/internal/httputil"
	allowedPrefixes := []string{"internal/distribution/okd/postinstall/"}

	root, err := findInternalDir()
	if err != nil {
		t.Fatalf("locate internal/: %v", err)
	}

	fset := token.NewFileSet()
	var violations []string

	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		imp, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return nil
		}
		if !importsPath(imp, importPath) {
			return nil
		}
		full, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil
		}
		if !callsNewInsecure(full) {
			return nil
		}
		rel := filepath.ToSlash(path)
		ok := false
		for _, prefix := range allowedPrefixes {
			if strings.Contains(rel, prefix) {
				ok = true
				break
			}
		}
		if !ok {
			violations = append(violations, path)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk: %v", walkErr)
	}
	for _, v := range violations {
		t.Errorf("httputil.NewInsecure called outside allowed postinstall/ path: %s", v)
	}
}

func importsPath(f *ast.File, path string) bool {
	for _, spec := range f.Imports {
		if spec.Path != nil && strings.Trim(spec.Path.Value, `"`) == path {
			return true
		}
	}
	return false
}

func callsNewInsecure(f *ast.File) bool {
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if ident.Name == "httputil" && sel.Sel.Name == "NewInsecure" {
			found = true
			return false
		}
		return true
	})
	return found
}

func findInternalDir() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, "internal")
		if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
