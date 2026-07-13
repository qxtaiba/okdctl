package setup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

var setupStepOrder = []distribution.StepID{
	StepInstallPackages,
	StepInstallTools,
	StepEnsureWorkDir,
	StepDownloadTools,
	StepGenerateConfig,
	StepGenerateManifests,
	StepGenerateKubeVIP,
	StepGenerateChrony,
	StepInjectManifests,
	StepCompactCluster,
	StepGenerateIgnition,
	StepInstallApache,
	StepDeployIgnition,
	StepVerifyWebServer,
	StepBuildISOs,
	StepUploadISOs,
	StepGenerateTfvars,
	StepConfigureHAProxy,
	StepConfigureFirewall,
	StepConfigureDNS,
}

func TestNewOptions_PathsRootedAtProject(t *testing.T) {
	opts := NewOptions(config.DefaultConfig(), "/proj")
	if opts.WorkDir != "/proj/okd-install" {
		t.Errorf("WorkDir = %q; want /proj/okd-install", opts.WorkDir)
	}
	if opts.DownloadDir != "/proj/okd-install/downloads" {
		t.Errorf("DownloadDir = %q; want /proj/okd-install/downloads", opts.DownloadDir)
	}
	if opts.TerraformEnv != "production" {
		t.Errorf("TerraformEnv = %q; want production (default)", opts.TerraformEnv)
	}
}

func TestSetupSteps_StepListAndSkipWiring(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(cfg *config.Config, opts *Options)
		wantSkip map[distribution.StepID]bool
	}{
		{
			// DefaultConfig has 3 workers, so the compact-cluster manifest
			// injection is skipped.
			name: "defaults",
			wantSkip: map[distribution.StepID]bool{
				StepDownloadTools:     false,
				StepCompactCluster:    true,
				StepBuildISOs:         false,
				StepUploadISOs:        false,
				StepConfigureHAProxy:  false,
				StepConfigureFirewall: false,
			},
		},
		{
			name:     "skip-downloads",
			mutate:   func(_ *config.Config, o *Options) { o.SkipDownloads = true },
			wantSkip: map[distribution.StepID]bool{StepDownloadTools: true},
		},
		{
			name:     "compact cluster runs manifest injection",
			mutate:   func(c *config.Config, _ *Options) { c.Topology.Workers.Count = 0 },
			wantSkip: map[distribution.StepID]bool{StepCompactCluster: false},
		},
		{
			name:     "skip-isos gates both build and upload",
			mutate:   func(_ *config.Config, o *Options) { o.SkipISOs = true },
			wantSkip: map[distribution.StepID]bool{StepBuildISOs: true, StepUploadISOs: true},
		},
		{
			name:     "skip-haproxy",
			mutate:   func(_ *config.Config, o *Options) { o.SkipHAProxy = true },
			wantSkip: map[distribution.StepID]bool{StepConfigureHAProxy: true, StepConfigureFirewall: false},
		},
		{
			name:     "skip-firewall",
			mutate:   func(_ *config.Config, o *Options) { o.SkipFirewall = true },
			wantSkip: map[distribution.StepID]bool{StepConfigureFirewall: true, StepConfigureHAProxy: false},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			opts := &Options{BaseOptions: phase.BaseOptions{WorkDir: t.TempDir(), ProjectRoot: t.TempDir()}}
			if tc.mutate != nil {
				tc.mutate(cfg, opts)
			}

			p := New(phase.WithLogger(logutil.NopLogger))
			defs := p.setupSteps(cfg, opts)

			if len(defs) != len(setupStepOrder) {
				t.Fatalf("step count = %d; want %d", len(defs), len(setupStepOrder))
			}
			byID := make(map[distribution.StepID]distribution.StepDef, len(defs))
			for i, d := range defs {
				if d.ID != setupStepOrder[i] {
					t.Errorf("step[%d] = %q; want %q", i, d.ID, setupStepOrder[i])
				}
				byID[d.ID] = d
			}
			for id, want := range tc.wantSkip {
				if got := byID[id].SkipWhen(); got != want {
					t.Errorf("%s: SkipWhen() = %v; want %v", id, got, want)
				}
			}
		})
	}
}

type recordingPkgManager struct {
	installs [][]string
}

func (m *recordingPkgManager) Install(_ context.Context, pkgs []string) error {
	m.installs = append(m.installs, pkgs)
	return nil
}

// installSetupFakeTools puts stub terraform/yq/helm/sops binaries and an
// argv-logging openshift-install on PATH. The fake openshift-install exits 0
// but writes no output files. Returns the argv log path.
func installSetupFakeTools(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake binaries rely on POSIX sh")
	}
	dir := t.TempDir()
	logPath := filepath.Join(dir, "argv.log")

	for _, name := range []string{"terraform", "yq", "helm", "sops"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	script := `#!/bin/sh
echo "$@" >> "$OC_ARGV_LOG"
exit 0
`
	if err := os.WriteFile(filepath.Join(dir, openshiftInstallBin), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("OC_ARGV_LOG", logPath)
	return logPath
}

// TestSetupExecute_ManifestPipeline drives Execute through the base and
// manifest steps. The fake openshift-install writes no .ign files, so the run
// stops deterministically at StepGenerateIgnition's validation — the web and
// infra tail (apache, webserver, ISO upload, haproxy, firewall, dns) shells
// to system services and Proxmox, too heavy to fake honestly; their ordering
// and skip wiring is carried by TestSetupSteps_StepListAndSkipWiring.
func TestSetupExecute_ManifestPipeline(t *testing.T) {
	argvLog := installSetupFakeTools(t)

	cfg := makeCfgWithFiles(t, `{"auths":{"registry.example.com":{"auth":"dXNlcjpwYXNz"}}}`, "ssh-ed25519 AAAA test@example")
	cfg.Topology.Workers.Count = 0
	cfg.Networking.Bastion.VIP = "192.168.1.100"

	projectRoot := t.TempDir()
	workDir := filepath.Join(projectRoot, "okd-install")
	opts := &Options{
		BaseOptions:   phase.BaseOptions{WorkDir: workDir, ProjectRoot: projectRoot},
		SkipDownloads: true,
	}

	p := New(phase.WithExecutor(executor.New(executor.WithLogger(logutil.NopLogger))), phase.WithLogger(logutil.NopLogger))
	p.Pkg = &recordingPkgManager{}

	results, err := p.Execute(context.Background(), cfg, opts)
	if err == nil {
		t.Fatal("expected ignition-validation error from fake openshift-install")
	}
	var cfgErr *errtypes.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Errorf("err = %v; want *errtypes.ConfigError", err)
	}

	wantResults := []struct {
		id      distribution.StepID
		success bool
		skipped bool
	}{
		{StepInstallPackages, true, false},
		{StepInstallTools, true, false},
		{StepEnsureWorkDir, true, false},
		{StepDownloadTools, true, true},
		{StepGenerateConfig, true, false},
		{StepGenerateManifests, true, false},
		{StepGenerateKubeVIP, true, false},
		{StepGenerateChrony, true, false},
		{StepInjectManifests, true, false},
		{StepCompactCluster, true, false},
		{StepGenerateIgnition, false, false},
	}
	if len(results) != len(wantResults) {
		t.Fatalf("result count = %d; want %d (run must stop at ignition validation)", len(results), len(wantResults))
	}
	for i, want := range wantResults {
		r := results[i]
		if r.StepID != want.id || r.Success != want.success || r.Skipped != want.skipped {
			t.Errorf("result[%d] = {%s success=%v skipped=%v}; want {%s success=%v skipped=%v}",
				i, r.StepID, r.Success, r.Skipped, want.id, want.success, want.skipped)
		}
	}

	clusterDir := phase.ClusterConfigDir(workDir)
	for _, f := range []string{
		filepath.Join(clusterDir, "install-config.yaml.backup"),
		ManifestsSentinel(clusterDir),
		filepath.Join(clusterDir, openshiftSubdir, "99-kube-vip-daemonset.yaml"),
		filepath.Join(clusterDir, openshiftSubdir, "99-ingress-controller-master-placement.yaml"),
		filepath.Join(clusterDir, openshiftSubdir, "99-master-chrony-configuration.yaml"),
		filepath.Join(clusterDir, openshiftSubdir, "99-worker-chrony-configuration.yaml"),
	} {
		if _, statErr := os.Stat(f); statErr != nil {
			t.Errorf("expected artifact missing: %v", statErr)
		}
	}

	data, readErr := os.ReadFile(argvLog)
	if readErr != nil {
		t.Fatalf("read argv log: %v", readErr)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	wantArgv := []string{
		"create manifests --dir " + clusterDir,
		"create ignition-configs --dir " + clusterDir,
	}
	if len(lines) != len(wantArgv) {
		t.Fatalf("openshift-install invocations = %d (%q); want %d", len(lines), lines, len(wantArgv))
	}
	for i, want := range wantArgv {
		if lines[i] != want {
			t.Errorf("openshift-install argv[%d] = %q; want %q", i, lines[i], want)
		}
	}
}
