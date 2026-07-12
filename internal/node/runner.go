package node

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/setup"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/system"
)

// Default gates for node-lifecycle waits. NodeReadyTimeout bounds the wait for
// a resized/rejoined node to report Ready; EtcdGateTimeout bounds the pre/post
// etcd-quorum health gate around a control-plane mutation.
const (
	DefaultNodeReadyTimeout = 15 * time.Minute
	DefaultEtcdGateTimeout  = 10 * time.Minute
	// hostMemoryReserveMiB is the hypervisor headroom the memory-budget guard
	// keeps free when projecting a resize (ZFS ARC, host services).
	hostMemoryReserveMiB = 2048
)

// Runner drives node-lifecycle ops against one cluster. TF mutates VMs;
// Cluster runs the Kubernetes lifecycle. ConfigPath and EnvDir are persisted
// on each op so a later full deploy reconciles to the same topology.
type Runner struct {
	Cluster     *cluster.Client
	TF          *terraform.Executor
	Cfg         *config.Config
	ConfigPath  string
	ProjectRoot string
	WorkDir     string
	EnvDir      string
	RunID       string
	DryRun      bool
	Log         *slog.Logger
	Reporter    logutil.ProgressReporter

	NodeReadyTimeout time.Duration
	EtcdGateTimeout  time.Duration
}

// NewRunner wires a Runner with derived work/env directories and default
// timeouts. cfg's terraform environment resolves the env directory.
func NewRunner(cl *cluster.Client, tf *terraform.Executor, cfg *config.Config, projectRoot, configPath, tfEnv, runID string, log *slog.Logger) *Runner {
	workDir := filepath.Join(projectRoot, "okd-install")
	return &Runner{
		Cluster:          cl,
		TF:               tf,
		Cfg:              cfg,
		ConfigPath:       configPath,
		ProjectRoot:      projectRoot,
		WorkDir:          workDir,
		EnvDir:           filepath.Join(projectRoot, "infrastructure", "terraform", "environments", tfEnv),
		RunID:            runID,
		Log:              logutil.OrNop(log),
		Reporter:         logutil.NopProgressReporter,
		NodeReadyTimeout: DefaultNodeReadyTimeout,
		EtcdGateTimeout:  DefaultEtcdGateTimeout,
	}
}

func (r *Runner) marker() string { return markerPath(r.WorkDir) }

// persistTopology re-renders terraform.tfvars from the mutated config and saves
// okdctl.yaml, so the reduced/resized topology survives into the next full
// deploy. WriteTerraformVars preserves the bootstrap sentinel (unlike deploy's
// GenerateTerraformVars) so a re-render cannot resurrect the bootstrap VM.
func (r *Runner) persistTopology() error {
	if err := setup.WriteTerraformVars(r.Cfg, r.EnvDir); err != nil {
		return fmt.Errorf("render terraform vars: %w", err)
	}
	if err := config.NewLoader().Save(r.Cfg, r.ConfigPath); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

// targetedApply plans a single-resource change, gates the plan to exactly
// (address, want), snapshots state, and applies the saved plan. A gate failure
// aborts before any mutation. Returns the state-snapshot path for diagnostics.
func (r *Runner) targetedApply(ctx context.Context, address string, want terraform.PlanAction) error {
	if err := r.TF.Init(ctx); err != nil {
		return r.TF.WithLockHint(&errtypes.ClusterError{Msg: "terraform init", Err: err})
	}

	planFile := "node-op.tfplan"
	planPath := filepath.Join(r.EnvDir, planFile)
	defer func() {
		if err := system.SafeRemove(planPath); err != nil {
			r.Log.Warn("node: plan file cleanup failed", "err", err)
		}
	}()

	if err := r.TF.Plan(ctx, terraform.PlanOptions{
		OutputPlanFile: planFile,
		Targets:        []string{address},
	}); err != nil {
		return r.TF.WithLockHint(&errtypes.ClusterError{Msg: "terraform plan", Err: err})
	}

	changes, err := r.TF.ShowPlanChanges(ctx, planPath)
	if err != nil {
		return &errtypes.ClusterError{Msg: "read terraform plan", Err: err}
	}
	if err := terraform.AssertOnlyChange(changes, address, want); err != nil {
		return &errtypes.ClusterError{Msg: "plan safety gate refused the change", Err: err}
	}

	if r.DryRun {
		r.Log.Info("node: dry-run — plan gate passed, skipping apply", "address", address, "action", string(want))
		return nil
	}

	snap, snapErr := r.TF.SnapshotState(ctx)
	if snapErr != nil {
		return &errtypes.ClusterError{Msg: "state snapshot", Err: snapErr}
	}

	if err := r.TF.Apply(ctx, terraform.ApplyOptions{PlanFile: planPath}); err != nil {
		msg := "terraform apply"
		if snap != "" {
			msg = fmt.Sprintf("terraform apply (state backup: %s)", snap)
		}
		return r.TF.WithLockHint(&errtypes.ClusterError{Msg: msg, Err: err})
	}
	return nil
}

// waitEtcdHealthy blocks until the etcd quorum is healthy or the gate times
// out. It is the mandatory pre/post gate around any control-plane mutation.
func (r *Runner) waitEtcdHealthy(ctx context.Context, phase string) error {
	var lastReason string
	ok := func(ctx context.Context) bool {
		h, err := r.Cluster.EtcdHealthy(ctx)
		if err != nil {
			lastReason = err.Error()
			return false
		}
		if !h.Healthy {
			lastReason = h.Reason
			return false
		}
		return true
	}
	if err := system.WaitForWithTimeout(ctx, "etcd", phase, ok, r.EtcdGateTimeout, r.Log); err != nil {
		return &errtypes.ClusterError{Msg: fmt.Sprintf("etcd health gate (%s) failed: %s", phase, lastReason), Err: err}
	}
	return nil
}

// waitNodeReady blocks until node reports Ready. Used after a resize apply so
// the next control-plane step never proceeds against a rebooting node.
func (r *Runner) waitNodeReady(ctx context.Context, node string) error {
	ok := func(ctx context.Context) bool {
		nodes, err := r.Cluster.ListNodes(ctx)
		if err != nil {
			return false
		}
		for _, n := range nodes {
			if n.Name == node {
				return n.Ready
			}
		}
		return false
	}
	if err := system.WaitForWithTimeout(ctx, "node", node+"-ready", ok, r.NodeReadyTimeout, r.Log); err != nil {
		return &errtypes.ClusterError{Msg: fmt.Sprintf("node %s did not become Ready", node), Err: err}
	}
	return nil
}
