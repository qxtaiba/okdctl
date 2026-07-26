package postinstall

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/addon"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/testutil"
)

var postinstallStepOrder = []distribution.StepID{
	StepVerifyHealth,
	StepVerifyKubeVIP,
	StepCleanupBootstrap,
	StepDeployProductionDNS,
	StepInstallAddons,
	StepDisableRHDefaults,
}

func TestPostinstallSteps_StepListAndSkipWiring(t *testing.T) {
	cases := []struct {
		name            string
		opts            Options
		kubeVIPVerified bool
		wantSkip        map[distribution.StepID]bool
	}{
		{
			name: "defaults: cleanup and dns gated on unverified kube-vip",
			wantSkip: map[distribution.StepID]bool{
				StepVerifyHealth:        false,
				StepVerifyKubeVIP:       false,
				StepCleanupBootstrap:    true,
				StepDeployProductionDNS: true,
			},
		},
		{
			name: "skip-cluster-health",
			opts: Options{SkipClusterHealth: true},
			wantSkip: map[distribution.StepID]bool{
				StepVerifyHealth: true,
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
			name: "defaults: disable-rh-defaults runs",
			wantSkip: map[distribution.StepID]bool{
				StepDisableRHDefaults: false,
			},
		},
		{
			name: "keep-redhat-catalogs",
			opts: Options{KeepRedHatCatalogs: true},
			wantSkip: map[distribution.StepID]bool{
				StepDisableRHDefaults: true,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{Cluster: config.ClusterConfig{Name: "test"}}
			p := New(phase.WithExecutor(executor.New()), phase.WithLogger(logutil.NopLogger))
			pctx := distribution.NewPhaseContext(postInstallContext{})
			if tc.kubeVIPVerified {
				pctx.Update(func(c *postInstallContext) { c.KubeVIPVerified = true })
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

	projectRoot := t.TempDir()
	envDir := seedBootstrapEnvDir(t, projectRoot)

	cfg := &config.Config{
		Cluster:    config.ClusterConfig{Name: "test"},
		Networking: config.NetworkingConfig{Bastion: config.BastionConfig{IP: "192.168.1.5"}},
	}
	opts := NewOptions(cfg, projectRoot)
	// Health, kube-vip verification, and disabling rh-subscription-gated
	// defaults all need a live cluster; their wiring is covered by
	// TestPostinstallSteps_StepListAndSkipWiring.
	opts.SkipClusterHealth = true
	opts.SkipKubeVIP = true
	opts.KeepRedHatCatalogs = true

	p := New(phase.WithExecutor(executor.New()), phase.WithLogger(logutil.NopLogger))
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
		StepVerifyKubeVIP:       true,
		StepDeployProductionDNS: true,
		StepInstallAddons:       false,
		StepDisableRHDefaults:   true,
	}
	for i, r := range results {
		if r.StepID != postinstallStepOrder[i] {
			t.Errorf("result[%d] = %q; want %q", i, r.StepID, postinstallStepOrder[i])
		}
		if !r.Success {
			t.Errorf("%s: Success = false; err = %v", r.StepID, r.Error)
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

	sentinel := filepath.Join(envDir, phase.BootstrapStateSentinelFile)
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
}

// installFakeTerraformArgv is the argv-logging sibling of
// installFakeTerraformForBootstrap for tests that assert the exact terraform
// command lines instead of exit-code behaviour.
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

// TestWarnIfDNSStranded asserts the update-ingress recovery hint fires exactly
// when the bootstrap VM is gone but production DNS never made it out.
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
