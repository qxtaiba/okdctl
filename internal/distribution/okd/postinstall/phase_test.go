package postinstall

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/addon"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/testutil"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

// newTestPhase builds a Phase with a real executor (fake binaries via PATH) and the no-op logger.
func newTestPhase(t *testing.T) *Phase {
	t.Helper()
	return New(phase.WithExecutor(executor.New()), phase.WithLogger(logutil.NopLogger))
}

var postinstallStepOrder = []distribution.StepID{
	StepVerifyHealth,
	StepVerifyKubeVIP,
	StepCleanupBootstrap,
	StepStopIgnitionServer,
	StepDeployProductionDNS,
	StepInstallAddons,
	StepDisableRHDefaults,
}

func TestPostinstallSteps_StepListAndSkipWiring(t *testing.T) {
	cases := []struct {
		name            string
		opts            Options
		kubeVIPVerified bool
		bootstrapGone   bool
		wantSkip        map[distribution.StepID]bool
	}{
		{
			name: "defaults: cleanup and dns gated on unverified kube-vip",
			wantSkip: map[distribution.StepID]bool{
				StepVerifyHealth:        false,
				StepVerifyKubeVIP:       false,
				StepCleanupBootstrap:    true,
				StepStopIgnitionServer:  true,
				StepDeployProductionDNS: true,
				StepDisableRHDefaults:   false,
			},
		},
		{
			name: "skip-kubevip: bootstrap cleanup still runs",
			opts: Options{SkipKubeVIP: true},
			wantSkip: map[distribution.StepID]bool{
				StepVerifyKubeVIP:       true,
				StepCleanupBootstrap:    false,
				StepDeployProductionDNS: true,
			},
		},
		{
			name:            "verified kube-vip unlocks bootstrap cleanup and production dns",
			kubeVIPVerified: true,
			wantSkip: map[distribution.StepID]bool{
				StepCleanupBootstrap:    false,
				StepDeployProductionDNS: false,
			},
		},
		{
			name:          "cleaned-up bootstrap unlocks the ignition server stop",
			bootstrapGone: true,
			wantSkip: map[distribution.StepID]bool{
				StepStopIgnitionServer: false,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{Cluster: config.ClusterConfig{Name: "test"}}
			p := newTestPhase(t)
			pctx := distribution.NewPhaseContext(postInstallContext{})
			if tc.kubeVIPVerified {
				pctx.Update(func(c *postInstallContext) { c.KubeVIPVerified = true })
			}
			if tc.bootstrapGone {
				pctx.Update(func(c *postInstallContext) { c.BootstrapCleaned = true })
			}
			mgr := addon.NewManager(cfg, addon.WithLogger(logutil.NopLogger))

			defs := p.postinstallSteps(cfg, &tc.opts, pctx, mgr)
			if len(defs) != len(postinstallStepOrder) {
				t.Fatalf("step count = %d; want %d", len(defs), len(postinstallStepOrder))
			}
			byID := make(map[distribution.StepID]distribution.StepDef, len(defs))
			for i, d := range defs {
				if d.ID != postinstallStepOrder[i] {
					t.Errorf("step[%d] = %q; want %q", i, d.ID, postinstallStepOrder[i])
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

func TestPostinstallExecute_BootstrapTeardownViaFakeTerraform(t *testing.T) {
	installFakeTerraformArgv(t)

	systemctlLog := installFakeSystemctl(t, "exit 1")

	projectRoot := t.TempDir()
	envDir := seedBootstrapEnvDir(t, projectRoot)
	webRoot := seedPublishedIgnition(t)

	cfg := &config.Config{
		Cluster:    config.ClusterConfig{Name: "test"},
		Networking: config.NetworkingConfig{Bastion: config.BastionConfig{IP: "192.168.1.5"}},
		HTTPServer: config.HTTPServerConfig{Root: webRoot},
	}
	opts := NewOptions(cfg, projectRoot)
	// Skip wiring for these is covered by TestPostinstallSteps_StepListAndSkipWiring.
	opts.SkipClusterHealth = true
	opts.SkipKubeVIP = true
	opts.KeepRedHatCatalogs = true

	p := newTestPhase(t)
	result, results, err := p.Execute(context.Background(), cfg, &opts)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(results) != len(postinstallStepOrder) {
		t.Fatalf("result count = %d; want %d", len(results), len(postinstallStepOrder))
	}
	wantSkipped := map[distribution.StepID]bool{
		StepVerifyHealth:        true,
		StepCleanupBootstrap:    false,
		StepStopIgnitionServer:  false,
		StepVerifyKubeVIP:       true,
		StepDeployProductionDNS: true,
		StepInstallAddons:       false,
		StepDisableRHDefaults:   true,
	}
	for i, r := range results {
		if r.StepID != postinstallStepOrder[i] {
			t.Errorf("result[%d] = %q; want %q", i, r.StepID, postinstallStepOrder[i])
		}
		if wantSuccess := r.StepID != StepStopIgnitionServer; r.Success != wantSuccess {
			t.Errorf("%s: Success = %v; want %v; err = %v", r.StepID, r.Success, wantSuccess, r.Error)
		}
		if r.Skipped != wantSkipped[r.StepID] {
			t.Errorf("%s: Skipped = %v; want %v", r.StepID, r.Skipped, wantSkipped[r.StepID])
		}
	}

	if !result.BootstrapCleaned {
		t.Error("Result.BootstrapCleaned = false; want true")
	}
	if result.DNSDeployed {
		t.Error("Result.DNSDeployed = true; want false (kube-vip unverified)")
	}
	if result.BastionIP != "192.168.1.5" {
		t.Errorf("Result.BastionIP = %q; want 192.168.1.5", result.BastionIP)
	}

	sentinel := filepath.Join(envDir, workspace.BootstrapStateSentinelFile)
	data, readErr := os.ReadFile(sentinel)
	if readErr != nil {
		t.Fatalf("bootstrap state sentinel not written: %v", readErr)
	}
	if got := string(data); got != `{"bootstrap_enabled": false}` {
		t.Errorf("sentinel content = %q; want bootstrap_enabled false", got)
	}

	lines := readBootstrapArgvLines(t)
	if len(lines) != 2 {
		t.Fatalf("terraform invocations = %d (%q); want 2 (plan, apply)", len(lines), lines)
	}
	for _, arg := range []string{
		"plan -lock-timeout=120s",
		"-var bootstrap_enabled=false",
		"-out=bootstrap-destroy.tfplan",
		"-target=module.okd_cluster.proxmox_virtual_environment_vm.bootstrap",
	} {
		if !strings.Contains(lines[0], arg) {
			t.Errorf("plan argv = %q; missing %q", lines[0], arg)
		}
	}
	if want := "apply -lock-timeout=120s " + filepath.Join(envDir, "bootstrap-destroy.tfplan"); lines[1] != want {
		t.Errorf("apply argv = %q; want %q", lines[1], want)
	}

	if got := publishedIgnitionFiles(t, webRoot); len(got) != 0 {
		t.Errorf("published ignition files after postinstall = %v; want none", got)
	}
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile(systemctlLog)
		if err != nil {
			t.Fatalf("read systemctl log: %v", err)
		}
		for _, want := range []string{"stop httpd", "disable httpd"} {
			if !strings.Contains(string(data), want) {
				t.Errorf("systemctl calls missing %q; got:\n%s", want, data)
			}
		}
	}
}

func TestPostinstallExecute_ReRunWithNothingPublishedSucceeds(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("systemctl branches are linux-only; darwin takes the GOOS gate")
	}
	installFakeTerraformArgv(t)
	installFakeSystemctl(t, `case "$1" in is-active) exit 1;; *) exit 0;; esac`)

	projectRoot := t.TempDir()
	seedBootstrapEnvDir(t, projectRoot)
	webRoot := t.TempDir()

	cfg := &config.Config{
		Cluster:    config.ClusterConfig{Name: "test"},
		HTTPServer: config.HTTPServerConfig{Root: webRoot},
	}
	opts := NewOptions(cfg, projectRoot)
	opts.SkipClusterHealth = true
	opts.SkipKubeVIP = true
	opts.KeepRedHatCatalogs = true

	p := newTestPhase(t)
	for _, round := range []string{"first", "resume"} {
		_, results, err := p.Execute(t.Context(), cfg, &opts)
		if err != nil {
			t.Fatalf("%s Execute: %v", round, err)
		}
		for _, r := range results {
			if r.StepID == StepStopIgnitionServer && (r.Skipped || !r.Success) {
				t.Errorf("%s: %s skipped=%v success=%v err=%v", round, r.StepID, r.Skipped, r.Success, r.Error)
			}
		}
	}
}

func installFakeSystemctl(t *testing.T, script string) string {
	t.Helper()
	callLog := filepath.Join(t.TempDir(), "systemctl.log")
	testutil.InstallFakeBin(t, "systemctl", "#!/bin/sh\necho \"$@\" >> '"+callLog+"'\n"+script+"\n")
	return callLog
}

func seedPublishedIgnition(t *testing.T) string {
	t.Helper()
	webRoot := t.TempDir()
	dir := filepath.Join(webRoot, "ignition")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"bootstrap.ign", "master.ign", "worker.ign"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return webRoot
}

func publishedIgnitionFiles(t *testing.T, webRoot string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(webRoot, "ignition"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read published ignition dir: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// installFakeTerraformArgv logs argv instead of switching on exit code.
func installFakeTerraformArgv(t *testing.T) {
	t.Helper()
	testutil.InstallFakeBin(t, "terraform", `#!/bin/sh
echo "$@" >> "$TF_ARGV_LOG"
exit 0
`)
	t.Setenv("TF_ARGV_LOG", filepath.Join(t.TempDir(), "argv.log"))
}

func readBootstrapArgvLines(t *testing.T) []string {
	t.Helper()
	//nolint:gosec // test reads its own t.Setenv-provided log path
	data, err := os.ReadFile(os.Getenv("TF_ARGV_LOG"))
	if err != nil {
		t.Fatalf("read argv log: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func TestWarnIfDNSStranded(t *testing.T) {
	cases := []struct {
		name     string
		state    postInstallContext
		wantWarn bool
	}{
		{"bootstrap gone, dns deployed", postInstallContext{BootstrapCleaned: true, KubeVIPVerified: true, DNSDeployed: true}, false},
		{"bootstrap gone, kubevip skipped, no dns", postInstallContext{BootstrapCleaned: true}, true},
		{"bootstrap gone, verified but dns failed", postInstallContext{BootstrapCleaned: true, KubeVIPVerified: true}, true},
		{"bootstrap kept", postInstallContext{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf strings.Builder
			p := New(phase.WithExecutor(executor.New()),
				phase.WithLogger(slog.New(slog.NewTextHandler(&buf, nil))))
			p.warnIfDNSStranded(tc.state)
			got := strings.Contains(buf.String(), "okdctl update-ingress")
			if got != tc.wantWarn {
				t.Errorf("warn emitted = %v; want %v (log: %s)", got, tc.wantWarn, buf.String())
			}
		})
	}
}
