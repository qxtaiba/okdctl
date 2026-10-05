package node

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

// backupOrderTF records the exact sequence of terraform operations so a
// "backup before mutation" regression shows up as an ordering failure against
// SnapshotState, not merely a does-a-backup-exist check.
type backupOrderTF struct {
	fakeTF
	order *[]string
}

func (f *backupOrderTF) Init(ctx context.Context) error {
	*f.order = append(*f.order, "init")
	return f.fakeTF.Init(ctx)
}

func (f *backupOrderTF) Plan(ctx context.Context, opts terraform.PlanOptions) error {
	*f.order = append(*f.order, "plan")
	return f.fakeTF.Plan(ctx, opts)
}

func (f *backupOrderTF) SnapshotState(ctx context.Context) (string, error) {
	*f.order = append(*f.order, "snapshot")
	return f.fakeTF.SnapshotState(ctx)
}

func (f *backupOrderTF) Apply(ctx context.Context, opts terraform.ApplyOptions) error {
	*f.order = append(*f.order, "apply")
	return f.fakeTF.Apply(ctx, opts)
}

func TestTargetedApply_BackupPrecedesFirstTerraformInvocation(t *testing.T) {
	var order []string
	ftf := &backupOrderTF{fakeTF: fakeTF{action: terraform.PlanActionUpdate}, order: &order}
	r := &Runner{
		TF:     ftf,
		Log:    logutil.NopLogger,
		envDir: t.TempDir(),
	}

	address := workerAddress(0)
	if err := r.targetedApply(context.Background(), address, terraform.PlanActionUpdate, nil, false); err != nil {
		t.Fatalf("targetedApply() = %v; want nil", err)
	}
	if len(order) == 0 {
		t.Fatal("no terraform operations recorded")
	}
	if order[0] != "snapshot" {
		t.Errorf("first terraform operation = %q; want %q — the state backup must precede every terraform invocation (init/plan included), not just apply", order[0], "snapshot")
	}
}

func TestNewRunner_OptionsDeriveDirsAndDefaults(t *testing.T) {
	cfg := config.DefaultConfig()
	projRoot := t.TempDir()
	configPath := filepath.Join(projRoot, "okdctl.yaml")
	r := NewRunner(
		nil, nil, cfg,
		WithLogger(nil),
		WithTerraformEnv("production"),
		WithProjectRoot(projRoot),
		WithConfigPath(configPath),
		WithRunID("run-42"),
	)

	if r.workDir != workspace.WorkDir(projRoot) {
		t.Errorf("workDir = %q; want derived from project root", r.workDir)
	}
	if r.envDir != workspace.TerraformEnvDir(projRoot, "production") {
		t.Errorf("envDir = %q; want derived from project root + tf env", r.envDir)
	}
	if r.Log == nil {
		t.Error("nil logger must normalize to a no-op logger")
	}
	if r.Reporter == nil {
		t.Error("Reporter default must be wired")
	}
	if r.NodeReadyTimeout != DefaultNodeReadyTimeout || r.EtcdGateTimeout != DefaultEtcdGateTimeout {
		t.Errorf("default timeouts not applied: %v %v", r.NodeReadyTimeout, r.EtcdGateTimeout)
	}
}
