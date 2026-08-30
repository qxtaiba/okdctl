package okd

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

func TestResolveIngressWorkDir(t *testing.T) {
	if got, want := resolveIngressWorkDir("/srv/proj", ""), "/srv/proj/okd-install"; got != want {
		t.Errorf("empty workdir: got %q, want %q", got, want)
	}
	if got, want := resolveIngressWorkDir("/srv/proj", "/explicit/dir"), "/explicit/dir"; got != want {
		t.Errorf("explicit workdir: got %q, want %q", got, want)
	}
}

// TestCLIDoesNotSetUpdateIngressWorkDir asserts runUpdateIngress leaves
// WorkDir unset — setting it to projectRoot pointed RemoveHAProxy at a
// nonexistent path and rolled back every cutover.
func TestCLIDoesNotSetUpdateIngressWorkDir(t *testing.T) {
	src := filepath.Join("..", "..", "cli", "update_ingress.go")
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, src, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", src, err)
	}
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		sel, ok := lit.Type.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "UpdateIngressOptions" {
			return true
		}
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "WorkDir" {
				t.Errorf("%s: UpdateIngressOptions literal sets WorkDir; leave it empty so the provisioner defaults it to <projectRoot>/okd-install",
					fset.Position(kv.Pos()))
			}
		}
		return true
	})
}
