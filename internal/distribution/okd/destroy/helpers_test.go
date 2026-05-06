package destroy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

func installFakeTerraform(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-terraform script relies on POSIX sh")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\ncase \"$1\" in\n  init) exit 0 ;;\n  *) echo \"fake terraform: $1 failed\" >&2; exit 1 ;;\nesac\n"
	path := filepath.Join(dir, "terraform")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func seedTerraformEnvDir(t *testing.T, projectRoot, env string) {
	t.Helper()
	envDir := filepath.Join(projectRoot, "infrastructure", "terraform", "environments", env)
	for _, sub := range []string{
		envDir,
		filepath.Join(envDir, ".terraform", "providers"),
	} {
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	stateFile := filepath.Join(envDir, "terraform.tfstate")
	// Populated state so HasState() returns true; tests that need an
	// empty-state behaviour seed their own tfstate.
	if err := os.WriteFile(stateFile, []byte(`{"version":4,"resources":[{"type":"x"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(envDir, ".terraform.lock.hcl"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
}

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

// TestDestroyInfrastructure_TFDestroyFails injects a fake terraform binary that
// exits 0 on init and 1 on all other subcommands. It locks that the returned
// error is *errtypes.ClusterError unwrapping to *executor.ExitError.
func TestDestroyInfrastructure_TFDestroyFails(t *testing.T) {
	installFakeTerraform(t)
	projectRoot := t.TempDir()
	seedTerraformEnvDir(t, projectRoot, "production")

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
		t.Fatal("expected error when terraform destroy fails")
	}

	var clusterErr *errtypes.ClusterError
	if !errors.As(err, &clusterErr) {
		t.Fatalf("err = %v; want *errtypes.ClusterError", err)
	}

	var execErr *executor.ExitError
	if !errors.As(err, &execErr) {
		t.Errorf("err = %v; want *executor.ExitError in chain", err)
	}
	if execErr != nil && execErr.ExitCode == 0 {
		t.Errorf("ExitCode = 0; want non-zero from failed plan")
	}
}
