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
	"github.com/qxtaiba/okdctl/internal/testutil"
)

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
	}

	if err := executeWithRecorder(context.Background(), opts, logutil.NopLogger, nil); err != nil {
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
	// Same invariant as infra_test.go, exercised through Execute's dispatch path.
	projectRoot := t.TempDir()
	tfstate := seedStateFile(t, projectRoot, "LIVE-STATE")

	opts := &Options{
		BaseOptions: phase.BaseOptions{
			WorkDir:      t.TempDir(),
			ProjectRoot:  projectRoot,
			TerraformEnv: "production",
		},
		Kind: TerraformOnly,
	}

	if err := executeWithRecorder(context.Background(), opts, logutil.NopLogger, nil); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	body, err := os.ReadFile(tfstate)
	if err != nil {
		t.Fatalf("terraform.tfstate removed (DATA LOSS): %v", err)
	}
	if string(body) != "LIVE-STATE" {
		t.Errorf("terraform.tfstate mutated: %q", body)
	}
}

// installFakePkg stubs rpm/dnf/dpkg/apt-get on PATH; dnf/apt-get log argv to pkg.called.
func installFakePkg(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	logScript := "#!/bin/sh\necho \"$@\" >> \"" + filepath.Join(dir, "pkg.called") + "\"\nexit 0\n"
	rpmScript := "#!/bin/sh\nexit 0\n"
	// dpkg -l <pkg> must print "ii  <pkg>" so platform's postCheck treats it as installed.
	dpkgScript := "#!/bin/sh\nfor a in \"$@\"; do\n  case \"$a\" in -*) continue ;; esac\n  echo \"ii  $a 1.0 amd64 fake\"\ndone\nexit 0\n"
	scripts := map[string]string{
		"dnf":     logScript,
		"apt-get": logScript,
		"rpm":     rpmScript,
		"dpkg":    dpkgScript,
	}
	for name, body := range scripts {
		testutil.InstallFakeBin(t, name, body)
	}
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
	}
}

func assertPkgCalled(t *testing.T, binDir, pkg string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(binDir, "pkg.called"))
	if err != nil {
		t.Fatalf("pkg manager was not called (pkg.called absent): %v", err)
	}
	if !strings.Contains(string(data), pkg) {
		t.Errorf("pkg manager called but %q not in args: %s", pkg, data)
	}
}

func assertPkgNotCalled(t *testing.T, binDir, pkg string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(binDir, "pkg.called"))
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		t.Fatalf("reading pkg.called: %v", err)
	}
	if strings.Contains(string(data), pkg) {
		t.Errorf("pkg manager called with %q but RemovePackages=false should skip Packages(): %s", pkg, data)
	}
}

func seedWorkAndWebDirs(t *testing.T) (workDir, httpRoot, ignFile string) {
	t.Helper()
	workDir = t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "probe.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	httpRoot = t.TempDir()
	ignDir := filepath.Join(httpRoot, "ignition")
	if err := os.MkdirAll(ignDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ignFile = filepath.Join(ignDir, "bootstrap.ign")
	if err := os.WriteFile(ignFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return workDir, httpRoot, ignFile
}

func TestExecute_FullKind_AllStepsRun(t *testing.T) {
	workDir, httpRoot, ignFile := seedWorkAndWebDirs(t)

	// Empty state avoids credential preservation triggered by populated/corrupt tfstate.
	const stateBody = `{"version":4,"resources":[]}`
	projectRoot := t.TempDir()
	tfstate := seedStateFile(t, projectRoot, stateBody)

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
	}

	if err := executeWithRecorder(context.Background(), opts, logutil.NopLogger, nil); err != nil {
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
	if string(body) != stateBody {
		t.Errorf("terraform.tfstate mutated: %q", body)
	}
}

func TestExecute_FullKind_AggregatesErrors(t *testing.T) {
	workDir, httpRoot, ignFile := seedWorkAndWebDirs(t)

	// A file where Terraform expects a directory yields ENOTDIR, not ENOENT, forcing a ConfigError.
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
	}

	err := executeWithRecorder(context.Background(), opts, logutil.NopLogger, nil)
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
		binDir := installFakePkg(t)
		opts := fullOptsWithFreshDirs(t)
		opts.RemovePackages = false
		opts.BinDir = binDir

		if err := executeWithRecorder(context.Background(), opts, logutil.NopLogger, nil); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		assertPkgNotCalled(t, binDir, "coreos-installer")
	})

	t.Run("true", func(t *testing.T) {
		binDir := installFakePkg(t)
		opts := fullOptsWithFreshDirs(t)
		opts.RemovePackages = true
		opts.BinDir = binDir

		if err := executeWithRecorder(context.Background(), opts, logutil.NopLogger, nil); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		assertPkgCalled(t, binDir, "coreos-installer")
	})
}

func TestExecute_PostDestroy_TFStateGating(t *testing.T) {
	setup := func(t *testing.T, tfstateBody string) (string, *Options) {
		t.Helper()
		projectRoot := t.TempDir()
		tfstate := seedStateFile(t, projectRoot, tfstateBody)
		// terraform.tfvars must exist so AlreadyDone returns false and the step runs.
		if err := os.WriteFile(filepath.Join(filepath.Dir(tfstate), "terraform.tfvars"), []byte("cluster_name = \"test\""), 0o600); err != nil {
			t.Fatal(err)
		}
		opts := &Options{
			BaseOptions: phase.BaseOptions{
				WorkDir:      t.TempDir(),
				ProjectRoot:  projectRoot,
				TerraformEnv: "production",
			},
			Kind:        TerraformOnly,
			PostDestroy: true,
		}
		return tfstate, opts
	}

	t.Run("non-empty state preserved", func(t *testing.T) {
		const body = `{"version":4,"resources":[{"type":"null_resource"}]}`
		tfstate, opts := setup(t, body)

		if err := executeWithRecorder(context.Background(), opts, logutil.NopLogger, nil); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		got, err := os.ReadFile(tfstate)
		if err != nil {
			t.Fatalf("terraform.tfstate removed (DATA LOSS): %v", err)
		}
		if string(got) != body {
			t.Errorf("terraform.tfstate mutated: %q", got)
		}
	})

	t.Run("empty state removed", func(t *testing.T) {
		tfstate, opts := setup(t, `{"version":4,"resources":[]}`)

		if err := executeWithRecorder(context.Background(), opts, logutil.NopLogger, nil); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if _, err := os.Stat(tfstate); !os.IsNotExist(err) {
			t.Errorf("empty terraform.tfstate not removed after PostDestroy: stat err = %v", err)
		}
	})
}
