package destroy

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/testutil"
)

func installFakeTerraform(t *testing.T) {
	t.Helper()
	script := "#!/bin/sh\ncase \"$1\" in\n  init) exit 0 ;;\n  *) echo \"fake terraform: $1 failed\" >&2; exit 1 ;;\nesac\n"
	testutil.InstallFakeBin(t, "terraform", script)
}

func TestStateLockHint(t *testing.T) {
	cases := []struct {
		name     string
		lockBody string   // "" seeds no lock file
		wantMsg  []string // substrings required in ConfigError.Msg; nil means want nil error
		wantDir  bool     // Msg must also embed the env dir path
	}{
		{"no lock file", "", nil, false},
		{"lock file present", `{"ID":"abc"}`, []string{"force-unlock", "abc"}, true},
		{"corrupt lock file", `not-json`, []string{"force-unlock", "<id>"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.lockBody != "" {
				if err := os.WriteFile(filepath.Join(dir, ".terraform.tfstate.lock.info"), []byte(tc.lockBody), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			err := terraform.New(dir).LockHint()
			if tc.wantMsg == nil {
				if err != nil {
					t.Errorf("expected nil; got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error; got nil")
			}
			var cfgErr *errtypes.ConfigError
			if !errors.As(err, &cfgErr) {
				t.Fatalf("err = %v; want *errtypes.ConfigError", err)
			}
			want := tc.wantMsg
			if tc.wantDir {
				want = append(want, dir)
			}
			for _, sub := range want {
				if !strings.Contains(cfgErr.Msg, sub) {
					t.Errorf("Msg = %q; want substring %q", cfgErr.Msg, sub)
				}
			}
		})
	}
}

func TestDestroyInfrastructure_MissingEnvDir(t *testing.T) {
	projectRoot := t.TempDir() // no infrastructure/terraform/environments/...
	p, opts := destroyPhaseFor(projectRoot)

	err := p.destroyInfrastructure(context.Background(), config.DefaultConfig(), opts)
	if err == nil {
		t.Fatal("expected error for missing env dir")
	}
	var cfgErr *errtypes.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Errorf("err = %v; want *errtypes.ConfigError", err)
	}
}

func TestDestroyInfrastructure_EmptyStateReturnsNil(t *testing.T) {
	projectRoot := t.TempDir()
	envDir := filepath.Join(projectRoot, "infrastructure", "terraform", "environments", "production")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}
	p, opts := destroyPhaseFor(projectRoot)

	if err := p.destroyInfrastructure(context.Background(), config.DefaultConfig(), opts); err != nil {
		t.Errorf("expected nil (no state = already destroyed); got %v", err)
	}
}

func TestDestroyInfrastructure_TFDestroyFails(t *testing.T) {
	installFakeTerraform(t)
	projectRoot := t.TempDir()
	seedDestroyEnvDir(t, projectRoot)
	p, opts := destroyPhaseFor(projectRoot)

	err := p.destroyInfrastructure(context.Background(), config.DefaultConfig(), opts)
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

func TestDestroyInfrastructure_CorruptState(t *testing.T) {
	cases := []struct {
		name    string
		withBak bool
	}{
		{"returns cluster error", false},
		{"bak names snapshot", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			projectRoot := t.TempDir()
			envDir := filepath.Join(projectRoot, "infrastructure", "terraform", "environments", "production")
			if err := os.MkdirAll(envDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(envDir, "terraform.tfstate"), []byte(`{not valid json`), 0o600); err != nil {
				t.Fatal(err)
			}
			wantSubstr := "corrupt"
			if tc.withBak {
				bakPath := filepath.Join(envDir, "terraform.tfstate.2024-06-01T00-00-00Z.bak")
				if err := os.WriteFile(bakPath, []byte(`{"version":4,"resources":[{"type":"x"}]}`), 0o600); err != nil {
					t.Fatal(err)
				}
				wantSubstr = bakPath
			}
			p, opts := destroyPhaseFor(projectRoot)

			err := p.destroyInfrastructure(context.Background(), config.DefaultConfig(), opts)
			if err == nil {
				t.Fatal("expected ClusterError for corrupt state; got nil")
			}
			var clusterErr *errtypes.ClusterError
			if !errors.As(err, &clusterErr) {
				t.Fatalf("err = %v; want *errtypes.ClusterError", err)
			}
			if !strings.Contains(clusterErr.Msg, wantSubstr) {
				t.Errorf("Msg = %q; want substring %q", clusterErr.Msg, wantSubstr)
			}
		})
	}
}

func TestCustomISONames(t *testing.T) {
	cases := []struct {
		name    string
		masters int
		workers int
		want    []string
	}{
		{
			name:    "multi master multi worker",
			masters: 3,
			workers: 2,
			want: []string{
				"bootstrap.iso",
				"master0.iso", "master1.iso", "master2.iso",
				"worker0.iso", "worker1.iso",
			},
		},
		{
			name:    "single master no workers",
			masters: 1,
			workers: 0,
			want:    []string{"bootstrap.iso", "master0.iso"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Topology.ControlPlane.Count = tc.masters
			cfg.Topology.Workers.Count = tc.workers

			got := customISONames(cfg)
			if !slices.Equal(got, tc.want) {
				t.Errorf("customISONames() = %v; want %v", got, tc.want)
			}
		})
	}
}

// installFakeStateList makes `state list <addr>` exit 0 only for addresses in present.
func installFakeStateList(t *testing.T, present ...string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("#!/bin/sh\nif [ \"$1\" != \"state\" ]; then exit 0; fi\ncase \"$3\" in\n")
	for _, addr := range present {
		b.WriteString("  '" + addr + "') echo \"$3\"; exit 0 ;;\n")
	}
	b.WriteString("  *) exit 1 ;;\nesac\n")
	testutil.InstallFakeBin(t, "terraform", b.String())
}

func driftConfig(masters, workers int) *config.Config {
	cfg := &config.Config{}
	cfg.Topology.ControlPlane.Count = masters
	cfg.Topology.Workers.Count = workers
	return cfg
}

func TestWarnTopologyDrift(t *testing.T) {
	const workerPastEnd = "module.okd_cluster.proxmox_virtual_environment_vm.worker[2]"

	cases := []struct {
		name     string
		present  []string
		scoped   bool
		wantWarn bool
		wantMsg  string
	}{
		{
			name:     "no drift stays silent",
			present:  nil,
			scoped:   true,
			wantWarn: false,
		},
		{
			name:     "scoped drift warns about surviving vms",
			present:  []string{workerPastEnd},
			scoped:   true,
			wantWarn: true,
			wantMsg:  "unscoped destroy",
		},
		{
			name:     "unscoped drift warns about orphan isos",
			present:  []string{workerPastEnd},
			scoped:   false,
			wantWarn: true,
			wantMsg:  "iso removal",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			installFakeStateList(t, tc.present...)
			h := &testutil.CaptureHandler{}
			p := newPhaseWithCapture(h)
			tf := terraform.New(t.TempDir(), terraform.WithLogger(logutil.NopLogger))

			p.warnTopologyDrift(context.Background(), tf, driftConfig(3, 2), tc.scoped)

			if got := h.HasLevel(slog.LevelWarn); got != tc.wantWarn {
				t.Fatalf("warn logged = %v; want %v", got, tc.wantWarn)
			}
			if tc.wantMsg != "" {
				rec, ok := h.Last()
				if !ok || !strings.Contains(rec.Message, tc.wantMsg) {
					t.Errorf("warn message = %q; want substring %q", rec.Message, tc.wantMsg)
				}
			}
		})
	}
}

func TestWarnTopologyDrift_ProbeFailureNeverBlocks(t *testing.T) {
	testutil.InstallFakeBin(t, "terraform", "#!/bin/sh\necho \"probe broken\" >&2\nexit 2\n")
	h := &testutil.CaptureHandler{}
	p := newPhaseWithCapture(h)
	tf := terraform.New(t.TempDir(), terraform.WithLogger(logutil.NopLogger))

	p.warnTopologyDrift(context.Background(), tf, driftConfig(1, 1), true)

	rec, ok := h.Last()
	if !ok {
		t.Fatal("probe failure must be logged")
	}
	if !strings.Contains(rec.Message, "drift probe failed") {
		t.Errorf("message = %q; want probe-failure diagnostic", rec.Message)
	}
}

// installProbingTerraform records per-subcommand whether the prevent_destroy
// override exists (cwd is the env dir; module dir is its ../../modules/proxmox-okd sibling).
func installProbingTerraform(t *testing.T, failSubcommand string) {
	t.Helper()
	testutil.InstallFakeBin(t, "terraform", `#!/bin/sh
if [ -f ../../modules/proxmox-okd/prevent_destroy_override.tf ]; then
  echo "$1 override-present" >> probe.log
else
  echo "$1 override-absent" >> probe.log
fi
if [ -n "${TF_FAKE_FAIL:-}" ] && [ "$1" = "$TF_FAKE_FAIL" ]; then
  echo 'Error: simulated failure' >&2
  exit 1
fi
exit 0
`)
	t.Setenv("TF_FAKE_FAIL", failSubcommand)
}

func seedModuleDir(t *testing.T, projectRoot string) string {
	t.Helper()
	moduleDir := filepath.Join(projectRoot, "infrastructure", "terraform", "modules", "proxmox-okd")
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return moduleDir
}

func destroyPhaseFor(projectRoot string) (*Phase, *Options) {
	p := &Phase{
		BasePhase: phase.NewBasePhase(
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
	return p, opts
}

func TestDestroyInfrastructure_TransientOverrideLifecycle(t *testing.T) {
	cases := []struct {
		name    string
		failSub string
		wantErr bool
	}{
		{"success removes override", "", false},
		{"terraform failure still removes override", "apply", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			installProbingTerraform(t, tc.failSub)
			projectRoot := t.TempDir()
			seedDestroyEnvDir(t, projectRoot)
			moduleDir := seedModuleDir(t, projectRoot)
			p, opts := destroyPhaseFor(projectRoot)

			err := p.destroyInfrastructure(context.Background(), config.DefaultConfig(), opts)
			if (err != nil) != tc.wantErr {
				t.Fatalf("destroyInfrastructure err = %v, wantErr = %v", err, tc.wantErr)
			}

			overridePath := filepath.Join(moduleDir, terraform.DestroyOverrideFileName)
			if _, statErr := os.Stat(overridePath); !os.IsNotExist(statErr) {
				t.Errorf("override must be removed after the destroy returns, stat err = %v", statErr)
			}

			probe, readErr := os.ReadFile(filepath.Join(projectRoot, "infrastructure", "terraform", "environments", "production", "probe.log"))
			if readErr != nil {
				t.Fatalf("fake terraform never ran: %v", readErr)
			}
			// The fake logs the probe before honoring TF_FAKE_FAIL.
			for _, want := range []string{"plan override-present", "apply override-present"} {
				if !strings.Contains(string(probe), want) {
					t.Errorf("probe log missing %q:\n%s", want, probe)
				}
			}
		})
	}
}

func TestDestroyInfrastructure_PreventDestroyHint(t *testing.T) {
	testutil.InstallFakeBin(t, "terraform", `#!/bin/sh
if [ "$1" = "plan" ]; then
  echo 'Error: Instance cannot be destroyed' >&2
  echo 'Resource module.okd_cluster.proxmox_virtual_environment_vm.extra has lifecycle.prevent_destroy set' >&2
  exit 1
fi
exit 0
`)
	projectRoot := t.TempDir()
	seedDestroyEnvDir(t, projectRoot)
	seedModuleDir(t, projectRoot)
	p, opts := destroyPhaseFor(projectRoot)

	err := p.destroyInfrastructure(context.Background(), config.DefaultConfig(), opts)
	if err == nil {
		t.Fatal("expected terraform failure")
	}
	if !strings.Contains(err.Error(), terraform.DestroyOverrideFileName) {
		t.Errorf("prevent_destroy failure must hint at the override recipe: %v", err)
	}
}
