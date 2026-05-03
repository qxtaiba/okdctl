package cleanup

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

// installFakeDNF prepends a temp dir containing rpm + dnf shell stubs to PATH.
// dnf appends its argv to dnf.called in the same dir; rpm always exits 0 so
// IsInstalled returns true for every package and Remove proceeds to dnf.
func installFakeDNF(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-dnf script relies on POSIX sh")
	}
	dir := t.TempDir()
	dnfScript := "#!/bin/sh\necho \"$@\" >> \"$(dirname \"$0\")/dnf.called\"\nexit 0\n"
	rpmScript := "#!/bin/sh\nexit 0\n"
	for name, body := range map[string]string{"dnf": dnfScript, "rpm": rpmScript} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

func fullOptsWithFreshDirs(t *testing.T) *Options {
	t.Helper()
	workDir := t.TempDir()
	projectRoot := t.TempDir()
	envDir := filepath.Join(projectRoot, "infrastructure", "terraform", "environments", "production")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return &Options{
		BaseOptions: phase.BaseOptions{
			WorkDir:      workDir,
			ProjectRoot:  projectRoot,
			TerraformEnv: "production",
		},
		Kind:           Full,
		HTTPServerRoot: t.TempDir(),
		HAProxyConfig:  filepath.Join(t.TempDir(), "haproxy.cfg"),
		Logger:         logutil.NopLogger,
	}
}

func assertDNFCalled(t *testing.T, binDir, pkg string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(binDir, "dnf.called"))
	if err != nil {
		t.Fatalf("dnf was not called (dnf.called absent): %v", err)
	}
	if !strings.Contains(string(data), pkg) {
		t.Errorf("dnf called but %q not in args: %s", pkg, data)
	}
}

func assertDNFNotCalled(t *testing.T, binDir, pkg string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(binDir, "dnf.called"))
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		t.Fatalf("reading dnf.called: %v", err)
	}
	if strings.Contains(string(data), pkg) {
		t.Errorf("dnf called with %q but RemovePackages=false should skip Packages(): %s", pkg, data)
	}
}

func TestExecute_FullKind_AllStepsRun(t *testing.T) {
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "probe.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	httpRoot := t.TempDir()
	ignDir := filepath.Join(httpRoot, "ignition")
	if err := os.MkdirAll(ignDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ignFile := filepath.Join(ignDir, "bootstrap.ign")
	if err := os.WriteFile(ignFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	projectRoot := t.TempDir()
	envDir := filepath.Join(projectRoot, "infrastructure", "terraform", "environments", "production")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tfstate := filepath.Join(envDir, "terraform.tfstate")
	if err := os.WriteFile(tfstate, []byte("STATE"), 0o600); err != nil {
		t.Fatal(err)
	}

	opts := &Options{
		BaseOptions: phase.BaseOptions{
			WorkDir:      workDir,
			ProjectRoot:  projectRoot,
			TerraformEnv: "production",
		},
		Kind:           Full,
		HTTPServerRoot: httpRoot,
		HAProxyConfig:  filepath.Join(t.TempDir(), "haproxy.cfg"),
		RemovePackages: false,
		Logger:         logutil.NopLogger,
	}

	if err := Execute(context.Background(), opts); err != nil {
		t.Errorf("Full kind errored unexpectedly: %v", err)
	}
	if _, err := os.Stat(workDir); !os.IsNotExist(err) {
		t.Errorf("WorkDirectory did not run (workDir still present): %v", err)
	}
	if _, err := os.Stat(ignFile); !os.IsNotExist(err) {
		t.Errorf("WebServer did not run (ignition file still present): %v", err)
	}
	body, err := os.ReadFile(tfstate)
	if err != nil {
		t.Fatalf("terraform.tfstate removed (DATA LOSS): %v", err)
	}
	if string(body) != "STATE" {
		t.Errorf("terraform.tfstate mutated: %q", body)
	}
}

func TestExecute_FullKind_AggregatesErrors(t *testing.T) {
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "probe.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	httpRoot := t.TempDir()
	ignDir := filepath.Join(httpRoot, "ignition")
	if err := os.MkdirAll(ignDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ignFile := filepath.Join(ignDir, "bootstrap.ign")
	if err := os.WriteFile(ignFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Plant a regular FILE where Terraform expects a directory. os.ReadDir
	// returns ENOTDIR (not ENOENT), so Terraform() returns *errtypes.ConfigError.
	projectRoot := t.TempDir()
	envsParent := filepath.Join(projectRoot, "infrastructure", "terraform")
	if err := os.MkdirAll(envsParent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(envsParent, "environments"), []byte("not-a-dir"), 0o600); err != nil {
		t.Fatal(err)
	}

	opts := &Options{
		BaseOptions: phase.BaseOptions{
			WorkDir:      workDir,
			ProjectRoot:  projectRoot,
			TerraformEnv: "",
		},
		Kind:           Full,
		HTTPServerRoot: httpRoot,
		HAProxyConfig:  filepath.Join(t.TempDir(), "haproxy.cfg"),
		RemovePackages: false,
		Logger:         logutil.NopLogger,
	}

	err := Execute(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error from Full kind with bad terraform environments path")
	}
	var cfgErr *errtypes.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Errorf("err = %v; want *errtypes.ConfigError in chain", err)
	}
	if _, statErr := os.Stat(workDir); !os.IsNotExist(statErr) {
		t.Errorf("WorkDirectory did not run (workDir still present): %v", statErr)
	}
	if _, statErr := os.Stat(ignFile); !os.IsNotExist(statErr) {
		t.Errorf("WebServer did not run (ignition file still present): %v", statErr)
	}
}

func TestExecute_FullKind_RemovePackagesGating(t *testing.T) {
	t.Run("false", func(t *testing.T) {
		binDir := installFakeDNF(t)
		opts := fullOptsWithFreshDirs(t)
		opts.RemovePackages = false
		opts.BinDir = binDir

		if err := Execute(context.Background(), opts); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		assertDNFNotCalled(t, binDir, "coreos-installer")
	})

	t.Run("true", func(t *testing.T) {
		binDir := installFakeDNF(t)
		opts := fullOptsWithFreshDirs(t)
		opts.RemovePackages = true
		opts.BinDir = binDir

		if err := Execute(context.Background(), opts); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		assertDNFCalled(t, binDir, "coreos-installer")
	})
}
