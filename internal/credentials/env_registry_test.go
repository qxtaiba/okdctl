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
	// deferredZeroize: the body must defer Zeroize/ZeroizeEnv on the
	// credentials receiver AND defer ZeroizeEnv on the distinct executor
	// built with the env slice, after the .Env() call.
	deferredZeroize zeroizeDiscipline = iota
	// manualZeroize: the env-bearing object escapes into a longer-lived
	// owner so a literal defer is impossible; the body must still call
	// ZeroizeEnv on that object (a receiver other than the credentials)
	// after the .Env() call, covering its early-error paths.
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
	envPos   token.Pos
	receiver string
}

type zeroizeCall struct {
	receiver string
	name     string
	deferred bool
	pos      token.Pos
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
		if v := disciplineViolation(discipline, s); v != "" {
			t.Errorf("%s: %s", s.key, v)
		}
	}

	for key := range allowedEnvCallSites {
		if !found[key] {
			t.Errorf("allowlist entry %q no longer matches a .Env() call site: remove or update it", key)
		}
	}
}

// TestZeroizeDisciplineChecks pins the checker itself against fixture
// bodies: each degenerate variant (executor defer missing, defer placed
// before the env slice exists, creds defer missing) must be reported, so
// the registry test cannot be satisfied by an unrelated Zeroize call.
func TestZeroizeDisciplineChecks(t *testing.T) {
	const deferredOK = `package p
func run() {
	creds := get()
	defer creds.Zeroize()
	tf := terraform.New(dir, terraform.WithEnv(creds.Env()))
	defer tf.ZeroizeEnv()
}`
	const deferredNoExec = `package p
func run() {
	creds := get()
	defer creds.Zeroize()
	tf := terraform.New(dir, terraform.WithEnv(creds.Env()))
	_ = tf
}`
	const deferredExecBeforeEnv = `package p
func run() {
	creds := get()
	defer creds.Zeroize()
	defer tf.ZeroizeEnv()
	tf.Configure(terraform.WithEnv(creds.Env()))
}`
	const deferredNoCreds = `package p
func run() {
	creds := get()
	tf := terraform.New(dir, terraform.WithEnv(creds.Env()))
	defer tf.ZeroizeEnv()
}`
	const manualOK = `package p
func run() error {
	creds := get()
	tf := terraform.New(dir, terraform.WithEnv(creds.Env()))
	if err := open(); err != nil {
		tf.ZeroizeEnv()
		return err
	}
	return nil
}`
	const manualOnlyCredsDefer = `package p
func run() error {
	creds := get()
	defer creds.Zeroize()
	tf := terraform.New(dir, terraform.WithEnv(creds.Env()))
	_ = tf
	return nil
}`

	tests := []struct {
		name          string
		src           string
		discipline    zeroizeDiscipline
		wantViolation bool
	}{
		{"deferred both defers present", deferredOK, deferredZeroize, false},
		{"deferred executor defer missing", deferredNoExec, deferredZeroize, true},
		{"deferred executor defer before env call", deferredExecBeforeEnv, deferredZeroize, true},
		{"deferred creds defer missing", deferredNoCreds, deferredZeroize, true},
		{"manual executor zeroize on error path", manualOK, manualZeroize, false},
		{"manual only creds defer", manualOnlyCredsDefer, manualZeroize, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := parseFixtureSite(t, tc.src)
			v := disciplineViolation(tc.discipline, s)
			if got := v != ""; got != tc.wantViolation {
				t.Errorf("violation = %q, wantViolation = %v", v, tc.wantViolation)
			}
		})
	}
}

func parseFixtureSite(t *testing.T, src string) envCallSite {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "fixture.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	sites := fileEnvCallSites(fset, f, "fixture.go")
	if len(sites) != 1 {
		t.Fatalf("fixture must contain exactly one .Env() call site, found %d", len(sites))
	}
	return sites[0]
}

func disciplineViolation(discipline zeroizeDiscipline, s envCallSite) string {
	if discipline == callerOwnedZeroize {
		return ""
	}
	if s.receiver == "" {
		return "cannot resolve the .Env() receiver; call Env on a plain identifier so the sweep can check its Zeroize"
	}
	calls := collectZeroizeCalls(s.fn.Body)
	if discipline == deferredZeroize {
		return deferredZeroizeViolation(s, calls)
	}
	return manualZeroizeViolation(s, calls)
}

func deferredZeroizeViolation(s envCallSite, calls []zeroizeCall) string {
	var credsDefer, execDefer bool
	for _, c := range calls {
		if !c.deferred {
			continue
		}
		if c.receiver == s.receiver {
			credsDefer = true
		} else if c.name == "ZeroizeEnv" && c.pos > s.envPos {
			execDefer = true
		}
	}
	switch {
	case !credsDefer:
		return "must defer Zeroize on the " + s.receiver + " receiver"
	case !execDefer:
		return "must defer ZeroizeEnv on the executor built with the env slice, after the .Env() call"
	}
	return ""
}

func manualZeroizeViolation(s envCallSite, calls []zeroizeCall) string {
	for _, c := range calls {
		if !c.deferred && c.name == "ZeroizeEnv" && c.receiver != s.receiver && c.pos > s.envPos {
			return ""
		}
	}
	return "must call ZeroizeEnv on the env-bearing object (early-error paths), after the .Env() call"
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
		if call, ok := findEnvCall(fn.Body); ok {
			sel := call.Fun.(*ast.SelectorExpr)
			sites = append(sites, envCallSite{
				key:      relPath + ":" + fn.Name.Name,
				position: fset.Position(call.Pos()).String(),
				fn:       fn,
				envPos:   call.Pos(),
				receiver: receiverName(sel.X),
			})
		}
	}
	return sites
}

func findEnvCall(body *ast.BlockStmt) (envCall *ast.CallExpr, found bool) {
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 0 {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Env" {
			envCall = call
			found = true
			return false
		}
		return true
	})
	return envCall, found
}

func collectZeroizeCalls(body *ast.BlockStmt) []zeroizeCall {
	var calls []zeroizeCall
	record := func(call *ast.CallExpr, deferred bool) {
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return
		}
		if sel.Sel.Name != "Zeroize" && sel.Sel.Name != "ZeroizeEnv" {
			return
		}
		calls = append(calls, zeroizeCall{
			receiver: receiverName(sel.X),
			name:     sel.Sel.Name,
			deferred: deferred,
			pos:      call.Pos(),
		})
	}
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.DeferStmt:
			record(node.Call, true)
			return false
		case *ast.CallExpr:
			record(node, false)
		}
		return true
	})
	return calls
}

func receiverName(expr ast.Expr) string {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		if base := receiverName(x.X); base != "" {
			return base + "." + x.Sel.Name
		}
	}
	return ""
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
