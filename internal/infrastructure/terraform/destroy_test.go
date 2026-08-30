package terraform

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/testutil"
)

// installRecordingTerraform logs each invocation's argv to argv.log in workDir;
// failSubcommand exits 1 for that subcommand.
func installRecordingTerraform(t *testing.T, failSubcommand string) {
	t.Helper()
	testutil.InstallFakeBin(t, "terraform", `#!/bin/sh
printf '%s\n' "$*" >> argv.log
if [ -n "${TF_FAKE_FAIL:-}" ] && [ "$1" = "$TF_FAKE_FAIL" ]; then
  echo 'Error: simulated failure' >&2
  exit 1
fi
exit 0
`)
	t.Setenv("TF_FAKE_FAIL", failSubcommand)
}

func recordedArgv(t *testing.T, workDir string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(workDir, "argv.log"))
	if err != nil {
		t.Fatalf("read argv.log: %v", err)
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

const masterAddr = "module.okd_cluster.proxmox_virtual_environment_vm.master[0]"

func TestExecutor_Destroy_UsePlanPlansThenAppliesSavedPlan(t *testing.T) {
	workDir := t.TempDir()
	installRecordingTerraform(t, "")
	tfvars := mustWriteFile(t, workDir, "terraform.tfvars", "x = 1\n")

	e := New(workDir)
	if err := e.Destroy(context.Background(), DestroyOptions{
		UsePlan: true,
		Targets: []string{masterAddr},
	}); err != nil {
		t.Fatalf("Destroy: %v", err)
	}

	lines := recordedArgv(t, workDir)
	if len(lines) != 2 {
		t.Fatalf("expected exactly plan+apply invocations, got %d: %v", len(lines), lines)
	}
	planFile := filepath.Join(workDir, "destroy.tfplan")
	wantPlan := "plan -lock-timeout=120s -var-file=" + tfvars + " -destroy -out=" + planFile + " -target=" + masterAddr
	if lines[0] != wantPlan {
		t.Errorf("destroy plan argv drifted:\n got %q\nwant %q", lines[0], wantPlan)
	}
	wantApply := "apply -lock-timeout=120s " + planFile
	if lines[1] != wantApply {
		t.Errorf("destroy apply argv drifted:\n got %q\nwant %q", lines[1], wantApply)
	}
}

func TestExecutor_Destroy_PlanFailureNeverMutates(t *testing.T) {
	workDir := t.TempDir()
	installRecordingTerraform(t, "plan")

	e := New(workDir)
	err := e.Destroy(context.Background(), DestroyOptions{UsePlan: true})
	if err == nil {
		t.Fatal("expected error when destroy plan fails")
	}
	if !strings.Contains(err.Error(), "destroy plan:") {
		t.Errorf("error should name the failed plan step: %v", err)
	}

	for _, line := range recordedArgv(t, workDir) {
		if strings.HasPrefix(line, "apply") || strings.HasPrefix(line, "destroy") {
			t.Fatalf("plan failure must not reach a mutating subcommand; ran: %q", line)
		}
	}
}

// destroyDirect has no production caller; this pins its argv shape as the
// regression coverage its doc comment relies on.
func TestExecutor_DestroyDirect_ArgvShape(t *testing.T) {
	workDir := t.TempDir()
	installRecordingTerraform(t, "")
	tfvars := mustWriteFile(t, workDir, "terraform.tfvars", "x = 1\n")

	e := New(workDir)
	if err := e.Destroy(context.Background(), DestroyOptions{
		AutoApprove: true,
		Parallelism: 4,
		Targets:     []string{masterAddr},
	}); err != nil {
		t.Fatalf("Destroy: %v", err)
	}

	lines := recordedArgv(t, workDir)
	if len(lines) != 1 {
		t.Fatalf("direct destroy must be a single invocation, got %d: %v", len(lines), lines)
	}
	want := "destroy -lock-timeout=120s -var-file=" + tfvars + " -auto-approve -parallelism=4 -target=" + masterAddr
	if lines[0] != want {
		t.Errorf("direct destroy argv drifted:\n got %q\nwant %q", lines[0], want)
	}
}

func TestExecutor_Apply_PlanFileSupersedesVarsAndApprove(t *testing.T) {
	workDir := t.TempDir()
	installRecordingTerraform(t, "")
	planFile := filepath.Join(workDir, "destroy.tfplan")

	e := New(workDir)
	if err := e.Apply(context.Background(), ApplyOptions{
		PlanFile:    planFile,
		AutoApprove: true,
		Vars:        map[string]string{"worker_count": "2"},
		Targets:     []string{masterAddr},
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	lines := recordedArgv(t, workDir)
	want := "apply -lock-timeout=120s " + planFile
	if len(lines) != 1 || lines[0] != want {
		t.Errorf("plan-file apply argv drifted:\n got %v\nwant [%q]", lines, want)
	}
}

func TestExecutor_Init_RefusesForeignStateMajor(t *testing.T) {
	workDir := t.TempDir()
	installRecordingTerraform(t, "")
	mustWriteFile(t, workDir, "terraform.tfstate", `{"version":4,"terraform_version":"2.1.0","resources":[{"type":"x"}]}`)

	e := New(workDir)
	err := e.Init(context.Background())
	if err == nil {
		t.Fatal("expected refusal for terraform_version major 2 state")
	}
	if !strings.Contains(err.Error(), "major 2") {
		t.Errorf("error should name the offending major: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(workDir, "argv.log")); !os.IsNotExist(statErr) {
		t.Error("version refusal must precede any terraform invocation")
	}
}

func TestExecutor_Init_AlreadyInitializedSkipsInvocation(t *testing.T) {
	workDir := t.TempDir()
	installRecordingTerraform(t, "")
	if err := os.MkdirAll(filepath.Join(workDir, ".terraform", "providers"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, workDir, ".terraform.lock.hcl", "# lock\n")

	e := New(workDir)
	if err := e.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(workDir, "argv.log")); !os.IsNotExist(statErr) {
		t.Error("fully initialized workdir must not re-run terraform init")
	}
}

func TestExecutor_Init_PartialInitReinitializes(t *testing.T) {
	workDir := t.TempDir()
	installRecordingTerraform(t, "")
	// Partial init fixture: .terraform exists but lock file and providers dir are missing.
	if err := os.MkdirAll(filepath.Join(workDir, ".terraform"), 0o755); err != nil {
		t.Fatal(err)
	}

	e := New(workDir)
	if err := e.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	lines := recordedArgv(t, workDir)
	if len(lines) != 1 || lines[0] != "init" {
		t.Errorf("partial init must re-run terraform init, got %v", lines)
	}
}

func TestExecutor_ShowPlanChanges_FoldsReplace(t *testing.T) {
	workDir := t.TempDir()
	script := `#!/bin/sh
cat <<'EOF'
{"resource_changes":[
  {"address":"` + masterAddr + `","change":{"actions":["delete","create"]}},
  {"address":"module.okd_cluster.proxmox_virtual_environment_vm.worker[0]","change":{"actions":["no-op"]}}
]}
EOF
exit 0
`
	testutil.InstallFakeBin(t, "terraform", script)

	e := New(workDir)
	changes, err := e.ShowPlanChanges(context.Background(), "destroy.tfplan")
	if err != nil {
		t.Fatalf("ShowPlanChanges: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 non-noop change, got %d: %+v", len(changes), changes)
	}
	if changes[0].Address != masterAddr || changes[0].Action != PlanActionReplace {
		t.Errorf("expected %s replace, got %+v", masterAddr, changes[0])
	}
}

func TestExecutor_ShowPlanChanges_NonZeroExitIsError(t *testing.T) {
	workDir := t.TempDir()
	testutil.InstallFakeBin(t, "terraform", "#!/bin/sh\necho 'Error: plan file not found' >&2\nexit 1\n")

	e := New(workDir)
	if _, err := e.ShowPlanChanges(context.Background(), "missing.tfplan"); err == nil {
		t.Fatal("expected error when terraform show fails")
	}
}

func TestExecutor_ZeroizeEnv_NilSafe(t *testing.T) {
	(&Executor{}).ZeroizeEnv() // nil inner exec must not panic
	e := New(t.TempDir(), WithEnv([]string{"PROXMOX_VE_PASSWORD=secret"}))
	e.ZeroizeEnv()
}

// Pins the WithEnv→ZeroizeEnv delegation cli's defer tf.ZeroizeEnv() relies on.
func TestExecutor_ZeroizeEnv_BlanksInnerEnv(t *testing.T) {
	e := New(t.TempDir(), WithEnv([]string{
		"PROXMOX_VE_PASSWORD=hunter2",
		"PROXMOX_VE_API_TOKEN=tok",
	}))
	if snap := e.exec.SnapshotEnv(); len(snap) != 2 {
		t.Fatalf("WithEnv did not reach the inner executor: env len = %d, want 2", len(snap))
	}
	e.ZeroizeEnv()
	if snap := e.exec.SnapshotEnv(); len(snap) != 0 {
		t.Errorf("ZeroizeEnv left %d env entries reachable: %q", len(snap), snap)
	}
}
