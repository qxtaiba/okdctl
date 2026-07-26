package install

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

var installStepOrder = []distribution.StepID{
	StepDeployInfra,
	StepWaitBootstrap,
	StepStartWorkers,
	StepSetupKubeconfig,
	StepValidateAccess,
	StepMonitorInstall,
	StepSetupAccess,
}

func TestNewOptions_TimeoutOverridesAndSSHKey(t *testing.T) {
	cfg := &config.Config{}
	opts := NewOptions(cfg, "/proj")
	if opts.BootstrapTimeout != DefaultBootstrapTimeout {
		t.Errorf("BootstrapTimeout = %v; want default %v", opts.BootstrapTimeout, DefaultBootstrapTimeout)
	}
	if opts.InstallTimeout != DefaultInstallTimeout {
		t.Errorf("InstallTimeout = %v; want default %v", opts.InstallTimeout, DefaultInstallTimeout)
	}
	if opts.WorkDir != "/proj/okd-install" {
		t.Errorf("WorkDir = %q; want /proj/okd-install", opts.WorkDir)
	}

	cfg.Deployment.BootstrapTimeout = 90
	cfg.Deployment.InstallTimeout = 120
	cfg.Files.SSHPublicKey = "/keys/id_ed25519.pub"
	opts = NewOptions(cfg, "/proj")
	if opts.BootstrapTimeout != 90*time.Second {
		t.Errorf("BootstrapTimeout = %v; want 90s override", opts.BootstrapTimeout)
	}
	if opts.InstallTimeout != 120*time.Second {
		t.Errorf("InstallTimeout = %v; want 120s override", opts.InstallTimeout)
	}
	if opts.SSHKeyPath != "/keys/id_ed25519" {
		t.Errorf("SSHKeyPath = %q; want private-key path with .pub trimmed", opts.SSHKeyPath)
	}
}

func TestInstallSteps_StepListAndSkipWiring(t *testing.T) {
	cases := []struct {
		name          string
		skipTerraform bool
		wantSkip      map[distribution.StepID]bool
	}{
		{
			name: "all enabled",
			wantSkip: map[distribution.StepID]bool{
				StepDeployInfra:  false,
				StepStartWorkers: false,
			},
		},
		{
			name:          "skip-terraform gates deploy and worker start",
			skipTerraform: true,
			wantSkip: map[distribution.StepID]bool{
				StepDeployInfra:  true,
				StepStartWorkers: true,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := New(phase.WithLogger(logutil.NopLogger))
			opts := &Options{
				BaseOptions:   phase.BaseOptions{WorkDir: t.TempDir(), ProjectRoot: t.TempDir()},
				SkipTerraform: tc.skipTerraform,
			}
			defs := p.installSteps(config.DefaultConfig(), opts)

			if len(defs) != len(installStepOrder) {
				t.Fatalf("step count = %d; want %d", len(defs), len(installStepOrder))
			}
			for i, d := range defs {
				if d.ID != installStepOrder[i] {
					t.Errorf("step[%d] = %q; want %q", i, d.ID, installStepOrder[i])
				}
			}
			for i, d := range defs {
				want, checked := tc.wantSkip[d.ID]
				if !checked {
					if d.SkipWhen != nil {
						t.Errorf("step[%d] %s: unexpected SkipWhen predicate", i, d.ID)
					}
					continue
				}
				if got := d.SkipWhen(); got != want {
					t.Errorf("%s: SkipWhen() = %v; want %v", d.ID, got, want)
				}
			}
		})
	}
}

// installExecuteFakes writes argv-logging openshift-install and oc scripts
// into one temp dir and prepends it to PATH. Returns the argv log path.
func installExecuteFakes(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake binaries rely on POSIX sh")
	}
	dir := t.TempDir()
	logPath := filepath.Join(dir, "argv.log")

	openshiftInstall := `#!/bin/sh
echo "openshift-install $@" >> "$OC_ARGV_LOG"
exit 0
`
	oc := `#!/bin/sh
echo "oc $@" >> "$OC_ARGV_LOG"
case "$1" in
  whoami) echo "system:admin" ;;
  version) echo "Server Version: 4.18.0" ;;
  get) echo '{"items":[]}' ;;
esac
exit 0
`
	for name, script := range map[string]string{"openshift-install": openshiftInstall, "oc": oc} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("OC_ARGV_LOG", logPath)
	return logPath
}

func TestInstallExecute_FullRunWithFakeBinaries(t *testing.T) {
	argvLog := installExecuteFakes(t)

	homeDir := t.TempDir()
	origHome := invokingUserHomeDirFn
	invokingUserHomeDirFn = func() (string, error) { return homeDir, nil }
	t.Cleanup(func() { invokingUserHomeDirFn = origHome })

	workDir := t.TempDir()
	clusterDir := workspace.ClusterConfigDir(workDir)
	if err := os.MkdirAll(filepath.Join(clusterDir, "auth"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(workspace.KubeconfigPath(clusterDir), []byte("apiVersion: v1\nkind: Config\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	p := New(
		phase.WithExecutor(executor.New(executor.WithLogger(logutil.NopLogger))),
		phase.WithLogger(logutil.NopLogger),
		phase.WithReporter(logutil.NopProgressReporter),
	)
	p.startMonitorCmd = func(_ context.Context, _ string) (<-chan error, func(), error) {
		done := make(chan error, 1)
		done <- nil
		return done, func() {}, nil
	}

	opts := &Options{
		BaseOptions:         phase.BaseOptions{WorkDir: workDir, ProjectRoot: t.TempDir()},
		SkipTerraform:       true, // provisioning needs a live Proxmox; step-list test carries its wiring
		BootstrapTimeout:    30 * time.Second,
		InstallTimeout:      30 * time.Second,
		CSRApprovalInterval: 10 * time.Millisecond,
	}

	results, err := p.Execute(context.Background(), config.DefaultConfig(), opts)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(results) != len(installStepOrder) {
		t.Fatalf("result count = %d; want %d", len(results), len(installStepOrder))
	}
	wantSkipped := map[distribution.StepID]bool{
		StepDeployInfra:  true,
		StepStartWorkers: true,
	}
	for i, r := range results {
		if r.StepID != installStepOrder[i] {
			t.Errorf("result[%d] = %q; want %q", i, r.StepID, installStepOrder[i])
		}
		if !r.Success {
			t.Errorf("%s: Success = false; err = %v", r.StepID, r.Error)
		}
		if r.Skipped != wantSkipped[r.StepID] {
			t.Errorf("%s: Skipped = %v; want %v", r.StepID, r.Skipped, wantSkipped[r.StepID])
		}
	}

	data, readErr := os.ReadFile(argvLog)
	if readErr != nil {
		t.Fatalf("read argv log: %v", readErr)
	}
	argv := string(data)
	wantBootstrapArgv := "openshift-install wait-for bootstrap-complete --dir " + clusterDir + " --log-level=debug"
	if !strings.Contains(argv, wantBootstrapArgv) {
		t.Errorf("argv log missing %q; got:\n%s", wantBootstrapArgv, argv)
	}
	if !strings.Contains(argv, "oc whoami") {
		t.Errorf("argv log missing oc whoami; got:\n%s", argv)
	}

	if _, statErr := os.Stat(filepath.Join(homeDir, ".kube", "config")); statErr != nil {
		t.Errorf("kubeconfig not installed into invoking user home: %v", statErr)
	}
}
