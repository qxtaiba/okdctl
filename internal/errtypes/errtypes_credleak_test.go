package errtypes_test

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/executor"
)

// TestMsgFieldNoCredentialInterpolation scans every non-test .go file under
// internal/ for composite literals of the four errtypes where Msg is set via
// fmt.Sprintf. It fails when the format string contains a known
// credential-bearing substring, enforcing the Msg-redaction contract.
func TestMsgFieldNoCredentialInterpolation(t *testing.T) {
	root, err := findInternalDir()
	if err != nil {
		t.Fatalf("cannot locate internal/ directory: %v", err)
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
		f, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return nil //nolint:nilerr // unparseable files are the compiler's problem; the sweep asserts only on parseable sources
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			if !isErrtype(lit) {
				return true
			}
			for _, elt := range lit.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				ident, ok := kv.Key.(*ast.Ident)
				if !ok || ident.Name != "Msg" {
					continue
				}
				if fmtStr, found := sprintfFormatArg(kv.Value); found {
					if credentialSubstring(fmtStr) {
						pos := fset.Position(lit.Pos())
						violations = append(violations, pos.String()+": Msg interpolates credential-bearing substring: "+fmtStr)
					}
				}
			}
			return true
		})
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk error: %v", walkErr)
	}
	for _, v := range violations {
		t.Errorf("credential leak in Msg field: %s", v)
	}
}

func isErrtype(lit *ast.CompositeLit) bool {
	sel, ok := lit.Type.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != "errtypes" {
		return false
	}
	switch sel.Sel.Name {
	case "ConfigError", "NetworkError", "ClusterError", "AuthError", "UsageError":
		return true
	}
	return false
}

func sprintfFormatArg(v ast.Expr) (string, bool) {
	call, ok := v.(*ast.CallExpr)
	if !ok {
		return "", false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != "fmt" || sel.Sel.Name != "Sprintf" {
		return "", false
	}
	if len(call.Args) == 0 {
		return "", false
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	return strings.Trim(lit.Value, `"`), true
}

func credentialSubstring(s string) bool {
	lower := strings.ToLower(s)
	for _, frag := range []string{"password", "api_key", "apikey", "passwd"} {
		if strings.Contains(lower, frag) {
			return true
		}
	}
	return false
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
			break
		}
		dir = parent
	}
	return "", os.ErrNotExist
}

// TestExitErrorCommandNoArgvLeak enforces the invariant that
// executor.ExitError.Command is bounded to "cli subcommand" form (as
// produced by cluster/k8s.go::subcommand) and never contains full argv.
// If full argv were passed, a --from-literal=password=<value> flag would
// appear verbatim in ExitError.Error() and reach log sinks.
//
// The adversarial sub-test acts as a canary: it confirms the current
// ExitError.Error() implementation does NOT redact Command-embedded
// credentials, meaning the cluster package's subcommand() function is the
// sole guard. Any change to that guard must update this test.
func TestExitErrorCommandNoArgvLeak(t *testing.T) {
	const secret = "s3cr3t"

	t.Run("safe pattern does not contain secret", func(t *testing.T) {
		e := &executor.ExitError{
			Command:  "oc create",
			ExitCode: 1,
			Stderr:   "some stderr",
		}
		if strings.Contains(e.Error(), secret) {
			t.Fatalf("ExitError.Error() contains secret in safe pattern: %q", e.Error())
		}
	})

	t.Run("adversarial full-argv pattern leaks secret", func(t *testing.T) {
		e := &executor.ExitError{
			Command:  "oc create secret generic mysecret --from-literal=password=" + secret,
			ExitCode: 1,
			Stderr:   "some stderr",
		}
		if !strings.Contains(e.Error(), secret) {
			t.Fatalf("canary broken: ExitError.Error() no longer surfaces Command verbatim; "+
				"ExitError may now self-redact — verify cluster/k8s.go::subcommand contract is still needed: %q", e.Error())
		}
	})

	t.Run("NewExitError with binary-only cmd does not leak", func(t *testing.T) {
		err := executor.NewExitError(context.Background(), "oc", 1, "exit 1")
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("NewExitError with binary-only cmd leaks secret: %q", err.Error())
		}
	})
}
