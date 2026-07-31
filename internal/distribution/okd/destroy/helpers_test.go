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
	tf := terraform.New(t.TempDir())
	if err := tf.LockHint(); err != nil {
		t.Errorf("expected nil; got %v", err)
	}
}

func TestStateLockHint_LockFilePresent(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, ".terraform.tfstate.lock.info")
	if err := os.WriteFile(lockPath, []byte(`{"ID":"abc"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	tf := terraform.New(dir)
	err := tf.LockHint()
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
	if !strings.Contains(cfgErr.Msg, "abc") {
		t.Errorf("Msg = %q; want lock ID 'abc' embedded", cfgErr.Msg)
	}
}

func TestStateLockHint_CorruptLockFile(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, ".terraform.tfstate.lock.info")
	if err := os.WriteFile(lockPath, []byte(`not-json`), 0o644); err != nil {
		t.Fatal(err)
	}
	tf := terraform.New(dir)
	err := tf.LockHint()
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
	if !strings.Contains(cfgErr.Msg, "<id>") {
		t.Errorf("Msg = %q; want fallback placeholder '<id>'", cfgErr.Msg)
	}
}

// TestDestroyInfrastructure_MissingEnvDir locks that missing env dir
// surfaces as a typed *ConfigError without attempting any subprocess.
func TestDestroyInfrastructure_MissingEnvDir(t *testing.T) {
	projectRoot := t.TempDir() // no infrastructure/terraform/environments/...

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

	err := p.destroyInfrastructure(context.Background(), config.DefaultConfig(), opts)
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

	if err := p.destroyInfrastructure(context.Background(), config.DefaultConfig(), opts); err != nil {
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

// TestDestroyInfrastructure_CorruptStateReturnsClusterError locks that a
// corrupt terraform.tfstate causes destroyInfrastructure to return a
// *errtypes.ClusterError rather than silently returning nil.
func TestDestroyInfrastructure_CorruptStateReturnsClusterError(t *testing.T) {
	projectRoot := t.TempDir()
	envDir := filepath.Join(projectRoot, "infrastructure", "terraform", "environments", "production")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(envDir, "terraform.tfstate"), []byte(`{not valid json`), 0o600); err != nil {
		t.Fatal(err)
	}

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

	err := p.destroyInfrastructure(context.Background(), config.DefaultConfig(), opts)
	if err == nil {
		t.Fatal("expected ClusterError for corrupt state; got nil")
	}
	var clusterErr *errtypes.ClusterError
	if !errors.As(err, &clusterErr) {
		t.Fatalf("err = %v; want *errtypes.ClusterError", err)
	}
	if !strings.Contains(clusterErr.Msg, "corrupt") {
		t.Errorf("Msg = %q; want substring 'corrupt'", clusterErr.Msg)
	}
}

// TestDestroyInfrastructure_CorruptStateWithBakNamesSnapshot locks that when
// a .bak snapshot exists alongside a corrupt tfstate, the ClusterError Msg
// embeds the snapshot path.
func TestDestroyInfrastructure_CorruptStateWithBakNamesSnapshot(t *testing.T) {
	projectRoot := t.TempDir()
	envDir := filepath.Join(projectRoot, "infrastructure", "terraform", "environments", "production")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(envDir, "terraform.tfstate"), []byte(`{not valid json`), 0o600); err != nil {
		t.Fatal(err)
	}
	bakName := "terraform.tfstate.2024-06-01T00-00-00Z.bak"
	bakPath := filepath.Join(envDir, bakName)
	if err := os.WriteFile(bakPath, []byte(`{"version":4,"resources":[{"type":"x"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

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

	err := p.destroyInfrastructure(context.Background(), config.DefaultConfig(), opts)
	if err == nil {
		t.Fatal("expected ClusterError for corrupt state; got nil")
	}
	var clusterErr *errtypes.ClusterError
	if !errors.As(err, &clusterErr) {
		t.Fatalf("err = %v; want *errtypes.ClusterError", err)
	}
	if !strings.Contains(clusterErr.Msg, bakPath) {
		t.Errorf("Msg = %q; want snapshot path %q embedded", clusterErr.Msg, bakPath)
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

// installFakeStateList installs a terraform stub whose `state list <addr>`
// exits 0 (present) only for addresses listed in present; every other
// address exits 1 with empty output (absent). Non-state subcommands exit 0.
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

func driftPhase(h *testutil.CaptureHandler) *Phase {
	return &Phase{
		BasePhase: phase.NewBasePhase(
			phase.WithExecutor(executor.New()),
			phase.WithLogger(slog.New(h)),
		),
	}
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
			p := driftPhase(h)
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

// TestWarnTopologyDrift_ProbeFailureNeverBlocks locks the best-effort
// contract: a broken probe (non-1 exit with stderr) logs and returns; the
// destroy itself is never gated on the diagnostic.
func TestWarnTopologyDrift_ProbeFailureNeverBlocks(t *testing.T) {
	testutil.InstallFakeBin(t, "terraform", "#!/bin/sh\necho \"probe broken\" >&2\nexit 2\n")
	h := &testutil.CaptureHandler{}
	p := driftPhase(h)
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

// installProbingTerraform installs a fake terraform that records, per
// subcommand, whether the transient prevent_destroy override exists in the
// module directory at invocation time (cwd is the env dir; the module dir is
// its ../../modules/proxmox-okd sibling). failSubcommand, when non-empty,
// makes that subcommand exit 1.
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

// TestDestroyInfrastructure_TransientOverrideLifecycle locks the blessed
// destroy path through the master prevent_destroy guard: the transient
// override exists while terraform's destroy plan and apply run, and is
// removed once the destroy returns — on success and on terraform failure
// alike — so it can never weaken a later non-destroy run.
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
			seedTerraformEnvDir(t, projectRoot, "production")
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
			// The fake logs the probe before honoring TF_FAKE_FAIL, so both
			// subcommands record the override as present in every case.
			for _, want := range []string{"plan override-present", "apply override-present"} {
				if !strings.Contains(string(probe), want) {
					t.Errorf("probe log missing %q:\n%s", want, probe)
				}
			}
		})
	}
}

// TestDestroyInfrastructure_PreventDestroyHint locks the error translation:
// when terraform still refuses on prevent_destroy (a resource okdctl's
// master override does not cover), the failure carries a hint naming the
// override path and recipe.
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
	seedTerraformEnvDir(t, projectRoot, "production")
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
