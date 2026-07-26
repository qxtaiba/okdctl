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

// envCallSite is one creds.Env() call: the file (repo-relative, in the
// registry's internal/-relative spelling) and the enclosing function.
type envCallSite struct {
	registryPath string
	funcName     string
	zeroizeOK    bool
}

// TestEnvCallSiteRegistry enforces the CLAUDE.md reviewer checklist as an
// invariant: every creds.Env() call site must (1) appear in the
// known-call-sites registry in the ProxmoxCredentials.Env doc comment and
// (2) pair with a ZeroizeEnv — either called in the enclosing function or
// explicitly delegated to callers via that function's doc comment. The
// inverse direction fails on stale registry entries whose call site moved.
func TestEnvCallSiteRegistry(t *testing.T) {
	root := findRepoRoot(t)
	sites := collectEnvCallSites(t, root)
	if len(sites) == 0 {
		t.Fatal("no creds.Env() call sites found; the sweep is broken")
	}

	registry := registryEntries(t)
	if len(registry) == 0 {
		t.Fatal("no .go entries parsed from the known-call-sites registry in proxmox.go")
	}

	for _, s := range sites {
		if !registry[s.registryPath] {
			t.Errorf("%s: creds.Env() call in %s is missing from the known-call-sites registry in ProxmoxCredentials.Env's doc comment (internal/credentials/proxmox.go)",
				s.registryPath, s.funcName)
		}
		if !s.zeroizeOK {
			t.Errorf("%s: function %s calls creds.Env() but neither calls ZeroizeEnv nor documents the caller-side ZeroizeEnv contract in its doc comment",
				s.registryPath, s.funcName)
		}
	}

	found := make(map[string]bool, len(sites))
	for _, s := range sites {
		found[s.registryPath] = true
	}
	for entry := range registry {
		if !found[entry] {
			t.Errorf("registry entry %q is stale: no creds.Env() call site exists there anymore", entry)
		}
	}
}

func collectEnvCallSites(t *testing.T, root string) []envCallSite {
	t.Helper()
	fset := token.NewFileSet()
	var sites []envCallSite

	for _, top := range []string{"internal", "cmd"} {
		dir := filepath.Join(root, top)
		walkErr := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			f, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if parseErr != nil {
				return nil //nolint:nilerr // unparseable files are the compiler's problem; the sweep asserts only on parseable sources
			}
			sites = append(sites, fileEnvCallSites(f, registryRelPath(root, path))...)
			return nil
		})
		if walkErr != nil {
			t.Fatalf("walk %s: %v", dir, walkErr)
		}
	}
	return sites
}

// registryRelPath converts an absolute path to the registry's spelling:
// relative to internal/ for internal packages (e.g. "cli/helpers.go"),
// repo-relative otherwise (e.g. "cmd/okdctl/main.go").
func registryRelPath(root, path string) string {
	if rel, err := filepath.Rel(filepath.Join(root, "internal"), path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

// fileEnvCallSites finds functions calling <cred-named-ident>.Env() with no
// args. Files aren't gated on a credentials import — the receiver's type
// usually arrives via a helper return value, so the import is often absent.
func fileEnvCallSites(f *ast.File, relPath string) []envCallSite {
	var sites []envCallSite
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		if !containsCredsEnvCall(fn.Body) {
			continue
		}
		zeroizeOK := containsZeroizeCall(fn.Body) ||
			(fn.Doc != nil && strings.Contains(fn.Doc.Text(), "ZeroizeEnv"))
		sites = append(sites, envCallSite{
			registryPath: relPath,
			funcName:     fn.Name.Name,
			zeroizeOK:    zeroizeOK,
		})
	}
	return sites
}

func containsCredsEnvCall(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Env" || len(call.Args) != 0 {
			return true
		}
		recv, ok := sel.X.(*ast.Ident)
		if ok && strings.Contains(strings.ToLower(recv.Name), "cred") {
			found = true
			return false
		}
		return true
	})
	return found
}

func containsZeroizeCall(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if ok && sel.Sel.Name == "ZeroizeEnv" {
			found = true
			return false
		}
		return true
	})
	return found
}

// registryEntries parses the .go paths out of the known-call-sites block in
// ProxmoxCredentials.Env's doc comment. Entry lines are tab-indented comment
// lines whose first token ends in ".go"; continuation lines don't match.
func registryEntries(t *testing.T) map[string]bool {
	t.Helper()
	data, err := os.ReadFile("proxmox.go")
	if err != nil {
		t.Fatalf("read proxmox.go: %v", err)
	}
	src := string(data)

	start := strings.Index(src, "Known call sites")
	if start < 0 {
		t.Fatal("proxmox.go: 'Known call sites' block not found in ProxmoxCredentials.Env doc comment")
	}
	end := strings.Index(src[start:], "func (c *ProxmoxCredentials) Env")
	if end < 0 {
		t.Fatal("proxmox.go: registry block is not directly above ProxmoxCredentials.Env")
	}

	entries := make(map[string]bool)
	for line := range strings.SplitSeq(src[start:start+end], "\n") {
		fields := strings.Fields(strings.TrimPrefix(strings.TrimSpace(line), "//"))
		if len(fields) > 0 && strings.HasSuffix(fields[0], ".go") {
			entries[fields[0]] = true
		}
	}
	return entries
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
