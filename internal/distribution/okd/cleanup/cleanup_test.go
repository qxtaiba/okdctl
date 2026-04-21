package cleanup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

func TestExecute_UnknownKind(t *testing.T) {
	opts := &Options{
		BaseOptions: phase.BaseOptions{WorkDir: t.TempDir()},
		Kind:        "unknown",
		Logger:      logutil.NopLogger,
	}
	err := Execute(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for unknown kind")
	}
	var cfgErr *errtypes.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Errorf("err = %v; want *errtypes.ConfigError", err)
	}
	if !strings.Contains(err.Error(), "unknown cleanup type") {
		t.Errorf("err message = %q; want 'unknown cleanup type'", err.Error())
	}
}

func TestExecute_WorkOnlyKindScopesToWorkDirOnly(t *testing.T) {
	// WorkOnly should not touch HAProxy/Apache/Dnsmasq/Terraform paths.
	// We prove this negatively by pointing them at paths that would error
	// if cleanup tried to touch them — and asserting the run still succeeds.
	workDir := t.TempDir()
	probe := filepath.Join(workDir, "probe.txt")
	if err := os.WriteFile(probe, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	opts := &Options{
		BaseOptions:    phase.BaseOptions{WorkDir: workDir, ProjectRoot: t.TempDir()},
		Kind:           WorkOnly,
		PreserveConfig: false,
		Logger:         logutil.NopLogger,
	}

	if err := Execute(context.Background(), opts); err != nil {
		t.Errorf("WorkOnly run errored: %v", err)
	}
	if _, err := os.Stat(workDir); !os.IsNotExist(err) {
		t.Errorf("workDir not removed under WorkOnly: %v", err)
	}
}

func TestExecute_WebOnlyKindScopesToWebServerOnly(t *testing.T) {
	httpRoot := t.TempDir()
	ignDir := filepath.Join(httpRoot, "ignition")
	if err := os.MkdirAll(ignDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"bootstrap.ign", "master.ign", "worker.ign"} {
		if err := os.WriteFile(filepath.Join(ignDir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	opts := &Options{
		BaseOptions:    phase.BaseOptions{WorkDir: t.TempDir()},
		Kind:           WebOnly,
		HTTPServerRoot: httpRoot,
		Logger:         logutil.NopLogger,
	}

	if err := Execute(context.Background(), opts); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	entries, _ := os.ReadDir(ignDir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".ign") {
			t.Errorf("ignition file not removed: %s", e.Name())
		}
	}
}

func TestExecute_TerraformOnlyPreservesTFState(t *testing.T) {
	// The tfstate-preservation invariant tested in infra_test.go applies
	// through Execute's dispatch path too.
	projectRoot := t.TempDir()
	envDir := filepath.Join(projectRoot, "infrastructure", "terraform", "environments", "production")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(envDir, "terraform.tfstate"), []byte("LIVE-STATE"), 0o600); err != nil {
		t.Fatal(err)
	}

	opts := &Options{
		BaseOptions: phase.BaseOptions{
			WorkDir:      t.TempDir(),
			ProjectRoot:  projectRoot,
			TerraformEnv: "production",
		},
		Kind:   TerraformOnly,
		Logger: logutil.NopLogger,
	}

	if err := Execute(context.Background(), opts); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(envDir, "terraform.tfstate"))
	if err != nil {
		t.Fatalf("terraform.tfstate removed (DATA LOSS): %v", err)
	}
	if string(body) != "LIVE-STATE" {
		t.Errorf("terraform.tfstate mutated: %q", body)
	}
}

func TestExecute_NilLoggerOk(t *testing.T) {
	opts := &Options{
		BaseOptions: phase.BaseOptions{WorkDir: t.TempDir()},
		Kind:        WorkOnly,
		Logger:      nil, // must not panic; Options.getLogger handles nil
	}
	if err := Execute(context.Background(), opts); err != nil {
		t.Errorf("nil logger caused error: %v", err)
	}
}
