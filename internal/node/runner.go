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

// clusterClient is the slice of cluster.Client the node ops drive. Defined as
// an interface so tests can substitute a call-recording fake (proving, e.g.,
// that a dry-run performs no cordon/drain) without a live cluster.
type clusterClient interface {
	ListNodes(ctx context.Context) ([]cluster.NodeDetail, error)
	Cordon(ctx context.Context, node string) error
	Uncordon(ctx context.Context, node string) error
	Drain(ctx context.Context, node string, opts cluster.DrainOptions) error
	DeleteNode(ctx context.Context, node string) error
	EtcdHealthy(ctx context.Context) (cluster.EtcdHealth, error)
	MastersSchedulable(ctx context.Context) (bool, error)
	SetMastersSchedulable(ctx context.Context, schedulable bool) error
	PodsForSelector(ctx context.Context, namespace, selector string) ([]cluster.PodPlacement, error)
	Apply(ctx context.Context, manifest []byte) error
}

// terraformExec is the slice of terraform.Executor node ops drive; an interface
// so a fake can record plan/apply calls without running terraform.
type terraformExec interface {
	Init(ctx context.Context) error
	Plan(ctx context.Context, opts terraform.PlanOptions) error
	ShowPlanChanges(ctx context.Context, planFile string) ([]terraform.ResourceChange, error)
	SnapshotState(ctx context.Context) (string, error)
	Apply(ctx context.Context, opts terraform.ApplyOptions) error
	WithLockHint(err error) error
}

// Runner drives node-lifecycle ops against one cluster. TF mutates VMs;
// Cluster runs the Kubernetes lifecycle. ConfigPath and EnvDir are persisted
// on each op so a later full deploy reconciles to the same topology.
type Runner struct {
	Cluster     clusterClient
	TF          terraformExec
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
// aborts before any mutation. planVars are plan-time -var overrides so the
// preview is truthful without persisting terraform.tfvars — the dry-run path
// relies on this to show the intended change while writing nothing to disk.
func (r *Runner) targetedApply(ctx context.Context, address string, want terraform.PlanAction, planVars map[string]string) error {
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

	// Post-deploy invariants every node op asserts: the bootstrap VM is gone and
	// workers run. Passed as -var (highest precedence, above terraform.tfvars and
	// auto-tfvars) so a stale terraform.tfvars bootstrap_enabled=true can't inject
	// a bootstrap-create into the targeted plan, and the module's
	// start_workers_immediately=false default can't plan a running worker to
	// stopped. Either would trip the single-change gate or, on apply, stop a
	// healthy VM. planVars (the caller's sizing) override these if they collide.
	vars := map[string]string{
		"bootstrap_enabled":         "false",
		"start_workers_immediately": "true",
	}
	for k, v := range planVars {
		vars[k] = v
	}

	if err := r.TF.Plan(ctx, terraform.PlanOptions{
		OutputPlanFile: planFile,
		Targets:        []string{address},
		Vars:           vars,
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
