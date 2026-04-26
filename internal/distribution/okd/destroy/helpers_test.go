package destroy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

func TestStateLockHint_NoLockFile(t *testing.T) {
	if err := stateLockHint(t.TempDir()); err != nil {
		t.Errorf("expected nil; got %v", err)
	}
}

func TestStateLockHint_LockFilePresent(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, ".terraform.tfstate.lock.info")
	if err := os.WriteFile(lockPath, []byte(`{"ID":"abc"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	err := stateLockHint(dir)
	if err == nil {
		t.Fatal("expected error; got nil")
	}
	var cfgErr *errtypes.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("err = %v; want *errtypes.ConfigError", err)
	}
	if !strings.Contains(cfgErr.Msg, "force-unlock") {
		t.Errorf("Msg = %q; want substring 'force-unlock'", cfgErr.Msg)
	}
	if !strings.Contains(cfgErr.Msg, dir) {
		t.Errorf("Msg = %q; want substring %q (dir)", cfgErr.Msg, dir)
	}
}

// TestDestroyInfrastructure_MissingEnvDir locks that missing env dir
// surfaces as a typed *ConfigError without attempting any subprocess.
func TestDestroyInfrastructure_MissingEnvDir(t *testing.T) {
	projectRoot := t.TempDir() // no infrastructure/terraform/environments/...

	p := &Phase{
		BasePhase: phase.NewBasePhase(
			"test",
			phase.WithExecutor(executor.New()),
			phase.WithLogger(logutil.NopLogger),
		),
	}
	opts := &Options{
		BaseOptions: phase.BaseOptions{
			ProjectRoot:  projectRoot,
			TerraformEnv: "production",
		},
		AutoApprove: true,
	}

	err := p.destroyInfrastructure(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for missing env dir")
	}
	var cfgErr *errtypes.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Errorf("err = %v; want *errtypes.ConfigError", err)
	}
}

// TestDestroyInfrastructure_EmptyStateReturnsNil locks that when the env
// dir exists but has no terraform.tfstate, destroyInfrastructure returns
// nil (the "already destroyed" fast path) without calling terraform.
func TestDestroyInfrastructure_EmptyStateReturnsNil(t *testing.T) {
	projectRoot := t.TempDir()
	envDir := filepath.Join(projectRoot, "infrastructure", "terraform", "environments", "production")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}

	p := &Phase{
		BasePhase: phase.NewBasePhase(
			"test",
			phase.WithExecutor(executor.New()),
			phase.WithLogger(logutil.NopLogger),
		),
	}
	opts := &Options{
		BaseOptions: phase.BaseOptions{
			ProjectRoot:  projectRoot,
			TerraformEnv: "production",
		},
		AutoApprove: true,
	}

	if err := p.destroyInfrastructure(context.Background(), opts); err != nil {
		t.Errorf("expected nil (no state = already destroyed); got %v", err)
	}
}
