package destroy

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
	"github.com/qxtaiba/okdctl/internal/distribution/okd/cleanup"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

var destroyStepOrder = []distribution.StepID{
	StepDestroyInfra,
	StepRemoveRemoteISO,
	StepCleanupFiles,
	StepCleanupFirewall,
	StepPrintSummary,
}

func assertStepIDs(t *testing.T, got, want []distribution.StepID) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("step count = %d; want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("step[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}

func resultIDs(results []distribution.StepResult) []distribution.StepID {
	ids := make([]distribution.StepID, len(results))
	for i, r := range results {
		ids[i] = r.StepID
	}
	return ids
}

func destroyableConfig() *config.Config {
	return &config.Config{
		Provider: config.ProviderConfig{
			Proxmox: &config.ProxmoxConfig{Host: "pve.example.com:8006", Node: "pve"},
		},
		Networking: config.NetworkingConfig{
			StaticIP: config.StaticIPConfig{Start: "192.168.1.10"},
		},
	}
}

func destroyableOpts(workDir, projectRoot string) *Options {
	return &Options{
		BaseOptions: phase.BaseOptions{
			WorkDir:      workDir,
			ProjectRoot:  projectRoot,
			TerraformEnv: "production",
		},
		AutoApprove: true,
		CleanupKind: cleanup.Full,
	}
}

func TestNewOptions_Defaults(t *testing.T) {
	cfg := &config.Config{}
	cfg.Deployment.AutoApprove = true
	opts := NewOptions(cfg, "/proj")

	if opts.WorkDir != "/proj/okd-install" {
		t.Errorf("WorkDir = %q; want /proj/okd-install", opts.WorkDir)
	}
	if opts.TerraformEnv != "production" {
		t.Errorf("TerraformEnv = %q; want production (default)", opts.TerraformEnv)
	}
	if !opts.AutoApprove {
		t.Error("AutoApprove not propagated from cfg.Deployment")
	}
	if opts.CleanupKind != cleanup.Full {
		t.Errorf("CleanupKind = %q; want %q", opts.CleanupKind, cleanup.Full)
	}
}

func TestDestroySteps_StepListAndSkipWiring(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(cfg *config.Config, opts *Options)
		tfFailed bool
		// wantSkip only lists steps with a deterministic expectation. The
		// firewall step's "not skipped" case depends on the host's active
		// firewall backend, so only its skip=true cases are asserted.
		wantSkip map[distribution.StepID]bool
	}{
		{
			name: "all enabled",
			wantSkip: map[distribution.StepID]bool{
				StepDestroyInfra:    false,
				StepRemoveRemoteISO: false,
				StepCleanupFiles:    false,
			},
		},
		{
			name:     "skip-terraform gates only the infra step",
			mutate:   func(_ *config.Config, o *Options) { o.SkipTerraform = true },
			wantSkip: map[distribution.StepID]bool{StepDestroyInfra: true, StepRemoveRemoteISO: false, StepCleanupFiles: false},
		},
		{
			name:     "keep-isos gates iso removal",
			mutate:   func(_ *config.Config, o *Options) { o.KeepISOs = true },
			wantSkip: map[distribution.StepID]bool{StepDestroyInfra: false, StepRemoveRemoteISO: true},
		},
		{
			name:     "nil proxmox provider gates iso removal",
			mutate:   func(c *config.Config, _ *Options) { c.Provider.Proxmox = nil },
			wantSkip: map[distribution.StepID]bool{StepRemoveRemoteISO: true},
		},
		{
			name:     "skip-cleanup gates file cleanup",
			mutate:   func(_ *config.Config, o *Options) { o.SkipCleanup = true },
			wantSkip: map[distribution.StepID]bool{StepCleanupFiles: true},
		},
		{
			name:     "empty cleanup kind gates file cleanup",
			mutate:   func(_ *config.Config, o *Options) { o.CleanupKind = "" },
			wantSkip: map[distribution.StepID]bool{StepCleanupFiles: true},
		},
		{
			name:     "absent workdir gates file cleanup",
			mutate:   func(_ *config.Config, o *Options) { o.WorkDir = filepath.Join(o.WorkDir, "does-not-exist") },
			wantSkip: map[distribution.StepID]bool{StepCleanupFiles: true},
		},
		{
			name:     "skip-firewall gates firewall cleanup",
			mutate:   func(_ *config.Config, o *Options) { o.SkipFirewall = true },
			wantSkip: map[distribution.StepID]bool{StepCleanupFirewall: true},
		},
		{
			name:     "terraform failure gates iso and firewall but not file cleanup",
			tfFailed: true,
			wantSkip: map[distribution.StepID]bool{
				StepRemoveRemoteISO: true,
				StepCleanupFiles:    false,
				StepCleanupFirewall: true,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := destroyableConfig()
			opts := destroyableOpts(t.TempDir(), t.TempDir())
			if tc.mutate != nil {
				tc.mutate(cfg, opts)
			}

			p := New(phase.WithExecutor(executor.New()), phase.WithLogger(logutil.NopLogger))
			defs := p.destroySteps(context.Background(), cfg, opts)

			ids := make([]distribution.StepID, len(defs))
			byID := make(map[distribution.StepID]distribution.StepDef, len(defs))
			for i, d := range defs {
				ids[i] = d.ID
				byID[d.ID] = d
			}
			assertStepIDs(t, ids, destroyStepOrder)

			if tc.tfFailed {
				defs[0].OnError(errors.New("boom"))
			}
			for id, want := range tc.wantSkip {
				if got := byID[id].SkipWhen(); got != want {
					t.Errorf("%s: SkipWhen() = %v; want %v", id, got, want)
				}
			}
		})
	}
}

// installFakeTerraformArgv writes an argv-logging POSIX-sh terraform into a
// temp dir and points PATH at ONLY that dir, which also keeps the firewall
// step deterministic: DetectBackend finds no firewall-cmd/ufw/iptables and
// reports None. TF_FAKE_MODE=plan-fail makes `terraform plan` exit 1.
func installFakeTerraformArgv(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-terraform script relies on POSIX sh")
	}
	dir := t.TempDir()
	logPath := filepath.Join(dir, "argv.log")
	script := `#!/bin/sh
echo "$@" >> "$TF_ARGV_LOG"
if [ "$1" = "state" ]; then
  # state list <addr> with no match: exit 1, empty stdout/stderr.
  exit 1
fi
if [ "$1" = "plan" ]; then
  if [ "${TF_FAKE_MODE:-ok}" = "plan-fail" ]; then
    echo "fake: plan error" >&2
    exit 1
  fi
  for a in "$@"; do
    case "$a" in
      -out=*) : > "${a#-out=}" ;;
    esac
  done
fi
if [ "$1" = "apply" ]; then
  # destroy-apply leaves an empty state, as real terraform destroy does.
  printf '{"version":4,"resources":[]}' > terraform.tfstate
fi
exit 0
`
	if err := os.WriteFile(filepath.Join(dir, "terraform"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("TF_ARGV_LOG", logPath)
	return logPath
}

func seedDestroyEnvDir(t *testing.T, projectRoot string) string {
	t.Helper()
	envDir := filepath.Join(projectRoot, "infrastructure", "terraform", "environments", "production")
	if err := os.MkdirAll(filepath.Join(envDir, ".terraform", "providers"), 0o755); err != nil {
		t.Fatal(err)
	}
	seed := map[string]string{
		".terraform.lock.hcl": "{}",
		"terraform.tfstate":   `{"version":4,"resources":[{"type":"proxmox_virtual_environment_vm","name":"node"}]}`,
		"terraform.tfvars":    "cluster_name = \"test\"\n",
	}
	for name, content := range seed {
		if err := os.WriteFile(filepath.Join(envDir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return envDir
}

func readArgvLines(t *testing.T, logPath string) []string {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read argv log: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func TestDestroyExecute_RunsTerraformDestroy(t *testing.T) {
	argvLog := installFakeTerraformArgv(t)
	projectRoot := t.TempDir()
	envDir := seedDestroyEnvDir(t, projectRoot)

	workDir := filepath.Join(projectRoot, "okd-install")
	if err := os.MkdirAll(filepath.Join(workDir, "downloads"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := destroyableConfig()
	cfg.Provider.Proxmox = nil // ISO removal requires live SSH to Proxmox; skipped here
	opts := destroyableOpts(workDir, projectRoot)
	opts.CleanupKind = cleanup.WorkOnly

	p := New(phase.WithExecutor(executor.New()), phase.WithLogger(logutil.NopLogger))
	results, err := p.Execute(context.Background(), cfg, opts)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	assertStepIDs(t, resultIDs(results), destroyStepOrder)
	wantSkipped := map[distribution.StepID]bool{
		StepDestroyInfra:    false,
		StepRemoveRemoteISO: true,
		StepCleanupFiles:    false,
		StepCleanupFirewall: true,
		StepPrintSummary:    false,
	}
	for _, r := range results {
		if !r.Success {
			t.Errorf("%s: Success = false; err = %v", r.StepID, r.Error)
		}
		if r.Skipped != wantSkipped[r.StepID] {
			t.Errorf("%s: Skipped = %v; want %v", r.StepID, r.Skipped, wantSkipped[r.StepID])
		}
	}

	lines := readArgvLines(t, argvLog)
	if len(lines) != 2 {
		t.Fatalf("terraform invocations = %d (%q); want 2 (plan, apply)", len(lines), lines)
	}
	planFile := filepath.Join(envDir, "destroy.tfplan")
	if !strings.HasPrefix(lines[0], "plan -lock-timeout=120s") {
		t.Errorf("plan argv = %q; want prefix %q", lines[0], "plan -lock-timeout=120s")
	}
	for _, arg := range []string{"-destroy", "-var-file=" + filepath.Join(envDir, "terraform.tfvars"), "-out=" + planFile} {
		if !strings.Contains(lines[0], arg) {
			t.Errorf("plan argv = %q; missing %q", lines[0], arg)
		}
	}
	if want := "apply -lock-timeout=120s " + planFile; lines[1] != want {
		t.Errorf("apply argv = %q; want %q", lines[1], want)
	}

	if _, statErr := os.Stat(workDir); !os.IsNotExist(statErr) {
		t.Errorf("work dir %s still exists after full-cleanup destroy", workDir)
	}
	if _, statErr := os.Stat(planFile); !os.IsNotExist(statErr) {
		t.Errorf("destroy.tfplan not cleaned up after destroy")
	}
	baks, globErr := filepath.Glob(filepath.Join(envDir, "terraform.tfstate.*.bak"))
	if globErr != nil || len(baks) == 0 {
		t.Errorf("no pre-destroy state snapshot written (err=%v)", globErr)
	}
}

func TestDestroyExecute_TerraformFailureStillReachesSummary(t *testing.T) {
	argvLog := installFakeTerraformArgv(t)
	t.Setenv("TF_FAKE_MODE", "plan-fail")

	projectRoot := t.TempDir()
	seedDestroyEnvDir(t, projectRoot)

	cfg := destroyableConfig()
	opts := destroyableOpts(filepath.Join(projectRoot, "okd-install"), projectRoot)

	p := New(phase.WithExecutor(executor.New()), phase.WithLogger(logutil.NopLogger))
	results, err := p.Execute(context.Background(), cfg, opts)
	if err == nil {
		t.Fatal("expected error when terraform destroy fails")
	}
	var clusterErr *errtypes.ClusterError
	if !errors.As(err, &clusterErr) {
		t.Errorf("err = %v; want *errtypes.ClusterError", err)
	}

	assertStepIDs(t, resultIDs(results), destroyStepOrder)
	if results[0].Success || results[0].Error == nil {
		t.Errorf("infra step: Success=%v Error=%v; want failed with error", results[0].Success, results[0].Error)
	}
	for _, i := range []int{1, 2, 3} {
		if !results[i].Skipped {
			t.Errorf("%s: Skipped = false; want true after terraform failure", results[i].StepID)
		}
	}
	if results[4].Success {
		t.Error("summary step reported success despite failed teardown")
	}

	if lines := readArgvLines(t, argvLog); len(lines) != 1 || !strings.HasPrefix(lines[0], "plan ") {
		t.Errorf("terraform argv = %q; want a single failed plan invocation", lines)
	}
}
