package terraform

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/testutil"
)

// installRecordingTerraform installs a fake terraform on PATH that appends
// each invocation's argv to argv.log in the executor's working directory
// (the executor sets cmd.Dir, so cwd == workDir). failSubcommand, when
// non-empty, makes that subcommand exit 1 with stderr output.
func installRecordingTerraform(t *testing.T, failSubcommand string) {
	t.Helper()
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> argv.log\n"
	if failSubcommand != "" {
		script += "if [ \"$1\" = \"" + failSubcommand + "\" ]; then echo 'Error: simulated failure' >&2; exit 1; fi\n"
	}
	script += "exit 0\n"
	testutil.InstallFakeBin(t, "terraform", script)
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
	tfvars := filepath.Join(workDir, "terraform.tfvars")
	if err := os.WriteFile(tfvars, []byte("x = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

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

// TestExecutor_Destroy_PlanFailureNeverMutates locks the no-silent-fallback
// contract: when the destroy plan fails, Destroy must surface the plan error
// and never degrade to a direct `terraform destroy` or apply — a plan failure
// usually signals an auth/state problem the operator must see before any
// infra mutation.
func TestExecutor_Destroy_PlanFailureNeverMutates(t *testing.T) {
	workDir := t.TempDir()
	installRecordingTerraform(t, "plan")

	e := New(workDir)
	err := e.Destroy(context.Background(), DestroyOptions{UsePlan: true})
	if err == nil {
		t.Fatal("expected error when destroy plan fails")
	}
	if !strings.Contains(err.Error(), "destroy plan failed") {
		t.Errorf("error should name the failed plan step: %v", err)
	}

	for _, line := range recordedArgv(t, workDir) {
		if strings.HasPrefix(line, "apply") || strings.HasPrefix(line, "destroy") {
			t.Fatalf("plan failure must not reach a mutating subcommand; ran: %q", line)
		}
	}
}

// TestExecutor_DestroyDirect_ArgvShape pins the emergency direct-destroy argv
// (UsePlan=false). destroyDirect currently has no production caller; this is
// the regression coverage its doc comment relies on.
func TestExecutor_DestroyDirect_ArgvShape(t *testing.T) {
	workDir := t.TempDir()
	installRecordingTerraform(t, "")
	tfvars := filepath.Join(workDir, "terraform.tfvars")
	if err := os.WriteFile(tfvars, []byte("x = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

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

// TestExecutor_Apply_PlanFileSupersedesVarsAndApprove locks ApplyOptions'
// documented mutual exclusion: with PlanFile set, Vars/VarFile/AutoApprove/
// Targets must not leak into argv — the saved plan already encodes the full
// change set, and appending -var would make terraform error out mid-destroy.
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

// TestExecutor_Init_RefusesForeignStateMajor locks the fail-closed preflight:
// a terraform.tfstate written by a different terraform major must abort Init
// before any terraform subprocess runs, so an incompatible CLI can never
// touch (and irreversibly upgrade or corrupt) the state.
func TestExecutor_Init_RefusesForeignStateMajor(t *testing.T) {
	workDir := t.TempDir()
	installRecordingTerraform(t, "")
	state := `{"version":4,"terraform_version":"2.1.0","resources":[{"type":"x"}]}`
	if err := os.WriteFile(filepath.Join(workDir, "terraform.tfstate"), []byte(state), 0o600); err != nil {
		t.Fatal(err)
	}

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
	if err := os.WriteFile(filepath.Join(workDir, ".terraform.lock.hcl"), []byte("# lock\n"), 0o600); err != nil {
		t.Fatal(err)
	}

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
	// .terraform exists but the lock file and providers dir are missing: a
	// crashed prior init. Init must re-run rather than trust the partial state.
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
