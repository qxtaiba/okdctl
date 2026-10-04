package addon

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/testutil"
)

// installStatefulFakeOC installs an "oc" stub that tracks namespace
// existence and labels in NS_STATE_DIR across calls, modeling a real
// cluster closely enough to reproduce ownership decisions across retries.
func installStatefulFakeOC(t *testing.T) string {
	t.Helper()
	stateDir := filepath.Join(t.TempDir(), "ns-state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.InstallFakeBin(t, "oc", "#!/bin/sh\n"+
		"mkdir -p \"$NS_STATE_DIR\"\n"+
		"cmd=\"$1\"\n"+
		"case \"$cmd\" in\n"+
		"get)\n"+
		"  ns=\"$3\"\n"+
		"  shift 3\n"+
		"  label=\"\"\n"+
		"  ignorenotfound=0\n"+
		"  while [ $# -gt 0 ]; do\n"+
		"    case \"$1\" in\n"+
		"      -l) shift; label=\"$1\" ;;\n"+
		"      --ignore-not-found) ignorenotfound=1 ;;\n"+
		"    esac\n"+
		"    shift\n"+
		"  done\n"+
		"  exists=0\n"+
		"  [ -f \"$NS_STATE_DIR/$ns.ns\" ] && exists=1\n"+
		"  if [ -n \"$label\" ]; then\n"+
		"    key=\"${label%%=*}\"\n"+
		"    val=\"${label#*=}\"\n"+
		"    labeled=0\n"+
		"    if [ \"$exists\" = 1 ] && grep -qxF \"$key=$val\" \"$NS_STATE_DIR/$ns.labels\" 2>/dev/null; then labeled=1; fi\n"+
		"    [ \"$labeled\" = 1 ] && echo \"namespace/$ns\"\n"+
		"    exit 0\n"+
		"  fi\n"+
		"  if [ \"$ignorenotfound\" = 1 ]; then\n"+
		"    [ \"$exists\" = 1 ] && echo \"namespace/$ns\"\n"+
		"    exit 0\n"+
		"  fi\n"+
		"  [ \"$exists\" = 1 ] && exit 0\n"+
		"  exit 1\n"+
		"  ;;\n"+
		"apply)\n"+
		"  manifest=\"$(cat)\"\n"+
		"  ns=$(printf '%s\\n' \"$manifest\" | awk '/^  name:/{print $2; exit}')\n"+
		"  : > \"$NS_STATE_DIR/$ns.ns\"\n"+
		"  printf '%s\\n' \"$manifest\" | awk '/labels:/{f=1;next} /^  name:/{f=0} f{gsub(/^ +/,\"\"); gsub(/: /,\"=\"); print}' > \"$NS_STATE_DIR/$ns.labels\"\n"+
		"  exit 0\n"+
		"  ;;\n"+
		"delete)\n"+
		"  resource=\"$2\"\n"+
		"  if [ \"$resource\" = \"namespace\" ]; then\n"+
		"    ns=\"$3\"\n"+
		"    rm -f \"$NS_STATE_DIR/$ns.ns\" \"$NS_STATE_DIR/$ns.labels\"\n"+
		"  fi\n"+
		"  exit 0\n"+
		"  ;;\n"+
		"esac\n"+
		"exit 0\n")
	return stateDir
}

func ownershipTestEnv(stateDir string) *Environment {
	return &Environment{
		Exec:   executor.New(executor.WithEnv([]string{"NS_STATE_DIR=" + stateDir})),
		Logger: logutil.NopLogger,
	}
}

// TestRollbackForNewNamespace_OwnershipSurvivesRetry reproduces the
// reviewer's cross-attempt scenario: a rollback that leaves debris behind
// must not cause the next attempt to misclassify that debris as foreign.
func TestRollbackForNewNamespace_OwnershipSurvivesRetry(t *testing.T) {
	stateDir := installStatefulFakeOC(t)
	env := ownershipTestEnv(stateDir)
	ctx := context.Background()
	const ns = "external-secrets"

	noopRollback := func(context.Context) error { return nil }

	rollback1, err := RollbackForNewNamespace(ctx, env, ns, noopRollback)
	if err != nil {
		t.Fatalf("first PrepareRollback: %v", err)
	}
	if rollback1 == nil {
		t.Fatal("first PrepareRollback denied rollback on an absent namespace")
	}

	if err := EnsureNamespace(ctx, env, ns); err != nil {
		t.Fatalf("simulate install creating namespace: %v", err)
	}

	if err := rollback1(ctx); err != nil {
		t.Fatalf("run granted rollback: %v", err)
	}

	owned, err := NamespaceOwnedByOkdctl(ctx, env, ns)
	if err != nil {
		t.Fatalf("inspect ownership after rollback: %v", err)
	}
	if !owned {
		t.Fatal("namespace debris from attempt 1 lost its ownership label")
	}

	rollback2, err := RollbackForNewNamespace(ctx, env, ns, noopRollback)
	if err != nil {
		t.Fatalf("second PrepareRollback: %v", err)
	}
	if rollback2 == nil {
		t.Fatal("second PrepareRollback misclassified okdctl's own debris as foreign — ownership signal degraded")
	}
}

// TestRollbackForNewNamespace_PreservesForeignNamespace proves the inverse
// direction: a namespace that exists but was never labeled by EnsureNamespace
// must never be granted for rollback, even though it already exists.
func TestRollbackForNewNamespace_PreservesForeignNamespace(t *testing.T) {
	stateDir := installStatefulFakeOC(t)
	env := ownershipTestEnv(stateDir)
	ctx := context.Background()
	const ns = "external-secrets"

	// A foreign namespace exists but was never created via EnsureNamespace,
	// so it carries no ownership label.
	if err := os.WriteFile(filepath.Join(stateDir, ns+".ns"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, ns+".labels"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	called := false
	rollback, err := RollbackForNewNamespace(ctx, env, ns, func(context.Context) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("PrepareRollback: %v", err)
	}
	if rollback != nil {
		t.Fatal("PrepareRollback granted rollback over a foreign namespace")
	}
	if called {
		t.Fatal("rollback must never run when ownership was denied")
	}

	owned, err := NamespaceOwnedByOkdctl(ctx, env, ns)
	if err != nil {
		t.Fatalf("inspect ownership: %v", err)
	}
	if owned {
		t.Fatal("foreign namespace misclassified as okdctl-owned")
	}
}

// TestEnsureNamespace_LabelsOnlyOnCreate proves EnsureNamespace does not
// retroactively stamp the ownership label onto an already-existing namespace.
func TestEnsureNamespace_LabelsOnlyOnCreate(t *testing.T) {
	stateDir := installStatefulFakeOC(t)
	env := ownershipTestEnv(stateDir)
	ctx := context.Background()
	const ns = "pre-existing"

	if err := os.WriteFile(filepath.Join(stateDir, ns+".ns"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, ns+".labels"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureNamespace(ctx, env, ns); err != nil {
		t.Fatalf("EnsureNamespace: %v", err)
	}

	owned, err := NamespaceOwnedByOkdctl(ctx, env, ns)
	if err != nil {
		t.Fatalf("inspect ownership: %v", err)
	}
	if owned {
		t.Fatal("EnsureNamespace labeled a namespace it did not create")
	}
}
