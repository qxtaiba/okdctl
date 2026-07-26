package httputil_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestNewInsecureCallerPolicy fails if any file both imports httputil and
// calls a TLS-skip-capable factory (NewInsecure, NewOptionalInsecure) from
// outside that factory's allowlisted paths. TLS-skip clients are legitimate
// only during the bootstrap window where no cluster CA is yet available
// (NewInsecure) or on the operator-opt-in Proxmox API paths
// (NewOptionalInsecure); new callers must add themselves to
// allowedPrefixes after a security review.
func TestNewInsecureCallerPolicy(t *testing.T) {
	const importPath = "github.com/qxtaiba/okdctl/internal/httputil"
	allowedPrefixes := map[string][]string{
		"NewInsecure": {"internal/distribution/okd/postinstall/"},
		"NewOptionalInsecure": {
			"internal/infrastructure/proxmox/",
			"internal/tui/wizard/steps/",
		},
	}

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
			return nil //nolint:nilerr // unparseable files are the compiler's problem; the sweep asserts only on parseable sources
		}
		if !importsPath(imp, importPath) {
			return nil
		}
		full, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil //nolint:nilerr // unparseable files are the compiler's problem; the sweep asserts only on parseable sources
		}
		rel := filepath.ToSlash(path)
		for _, fn := range calledInsecureFactories(full, allowedPrefixes) {
			ok := false
			for _, prefix := range allowedPrefixes[fn] {
				if strings.Contains(rel, prefix) {
					ok = true
					break
				}
			}
			if !ok {
				violations = append(violations, fmt.Sprintf("httputil.%s called outside its allowlisted paths: %s", fn, path))
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk: %v", walkErr)
	}
	for _, v := range violations {
		t.Error(v)
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

// calledInsecureFactories returns the gated httputil factory names (keys of
// gated) the file calls, deduplicated.
func calledInsecureFactories(f *ast.File, gated map[string][]string) []string {
	seen := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
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
		if ident.Name == "httputil" {
			if _, gatedFn := gated[sel.Sel.Name]; gatedFn {
				seen[sel.Sel.Name] = true
			}
		}
		return true
	})
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
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
