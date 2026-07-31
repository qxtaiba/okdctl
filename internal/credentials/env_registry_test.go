package credentials

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type zeroizeDiscipline int

const (
	// deferredZeroize: the function body must defer a Zeroize/ZeroizeEnv
	// call, bounding the plaintext lifetime to the enclosing frame.
	deferredZeroize zeroizeDiscipline = iota
	// manualZeroize: the env-bearing object escapes into a longer-lived
	// owner so a literal defer is impossible; the body must still call
	// ZeroizeEnv on its early-error paths.
	manualZeroize
	// callerOwnedZeroize: a constructor whose return value carries the env;
	// every caller owns the defer, so the body itself carries no Zeroize.
	callerOwnedZeroize
)

// allowedEnvCallSites is the registry of ProxmoxCredentials.Env() call
// sites, keyed "repo/relative/path.go:FuncName". Adding a call site means
// verifying its ZeroizeEnv hygiene per the Env doc comment and recording
// the discipline here.
var allowedEnvCallSites = map[string]zeroizeDiscipline{
	"internal/cli/destroy.go:runDestroyDryRun":        deferredZeroize,
	"internal/cli/helpers.go:runTerraformPlanPreview": deferredZeroize,
	"internal/cli/node.go:newRunner":                  manualZeroize,
	"internal/deploy/deploy.go:NewProvisioner":        callerOwnedZeroize,
}

type envCallSite struct {
	key      string
	position string
	fn       *ast.FuncDecl
}

// TestEnvCallSiteRegistry enforces the bounded-credential-lifetime
// convention statically: every non-test call of a zero-argument .Env()
// method must appear in allowedEnvCallSites, and each allowed site must
// honor its recorded Zeroize discipline. Matching is syntactic (no type
// resolution), which over-approximates — an unrelated Env() method would
// also trip the registry and force a human look, the fail-closed direction
// for a credential tripwire.
func TestEnvCallSiteRegistry(t *testing.T) {
	sites := collectEnvCallSites(t, findRepoRoot(t))
	if len(sites) == 0 {
		t.Fatal("no creds.Env() call sites found; the sweep is broken")
	}

	found := make(map[string]bool, len(sites))
	for _, s := range sites {
		found[s.key] = true

		discipline, ok := allowedEnvCallSites[s.key]
		if !ok {
			t.Errorf("unregistered .Env() call site at %s: verify its ZeroizeEnv hygiene per the ProxmoxCredentials.Env doc comment, then record %q in allowedEnvCallSites",
				s.position, s.key)
			continue
		}
		switch discipline {
		case deferredZeroize:
			if !containsZeroizeCall(s.fn.Body, true) {
				t.Errorf("%s must defer a Zeroize/ZeroizeEnv call in the same function body", s.key)
			}
		case manualZeroize:
			if !containsZeroizeCall(s.fn.Body, false) {
				t.Errorf("%s must call ZeroizeEnv on its early-error paths", s.key)
			}
		case callerOwnedZeroize:
		}
	}

	for key := range allowedEnvCallSites {
		if !found[key] {
			t.Errorf("allowlist entry %q no longer matches a .Env() call site: remove or update it", key)
		}
	}
}

func collectEnvCallSites(t *testing.T, root string) []envCallSite {
	t.Helper()
	fset := token.NewFileSet()
	var sites []envCallSite

	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if path != root && (strings.HasPrefix(name, ".") || name == "testdata" || name == "vendor" || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, parseErr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			return nil //nolint:nilerr // unparseable files are the compiler's problem; the sweep asserts only on parseable sources
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		sites = append(sites, fileEnvCallSites(fset, f, filepath.ToSlash(rel))...)
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk %s: %v", root, walkErr)
	}
	return sites
}

func fileEnvCallSites(fset *token.FileSet, f *ast.File, relPath string) []envCallSite {
	var sites []envCallSite
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		if pos, ok := findEnvCall(fset, fn.Body); ok {
			sites = append(sites, envCallSite{
				key:      relPath + ":" + fn.Name.Name,
				position: pos,
				fn:       fn,
			})
		}
	}
	return sites
}

func findEnvCall(fset *token.FileSet, body *ast.BlockStmt) (position string, found bool) {
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 0 {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Env" {
			position = fset.Position(call.Pos()).String()
			found = true
			return false
		}
		return true
	})
	return position, found
}

func containsZeroizeCall(body *ast.BlockStmt, deferredOnly bool) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		var call *ast.CallExpr
		switch node := n.(type) {
		case *ast.DeferStmt:
			call = node.Call
		case *ast.CallExpr:
			if deferredOnly {
				return true
			}
			call = node
		default:
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if sel.Sel.Name == "Zeroize" || sel.Sel.Name == "ZeroizeEnv" {
				found = true
			}
		}
		return true
	})
	return found
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the credentials package")
		}
		dir = parent
	}
}
