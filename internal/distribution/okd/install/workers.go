package install

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
)

// StartWorkerVMs starts worker VMs after bootstrap completes so they can reach the MCS.
//
// terraform.tfvars is the deploy-time snapshot written by setup.GenerateTerraformVars
// and is not mutated here. start_workers_immediately defaults to false in that
// snapshot; this call overrides it at apply time via -var. Operators running
// `terraform plan` from the workdir will see a diff on that variable — that is
// expected. The authoritative cluster state lives in tfstate, not in tfvars.
//
// The apply is scoped via -target to the worker VM resource only, matching
// CleanupBootstrap's precaution — an unscoped apply here would also
// reconcile any drift accumulated elsewhere (e.g. a hand-edited
// terraform.tfvars master CPU or network change) instead of leaving it for
// the next full apply. That drift is deferred, not lost — it surfaces on
// the next un-scoped `okdctl deploy` run.
func (p *Phase) StartWorkerVMs(ctx context.Context, cfg *config.Config, opts *Options) error {
	if cfg.Topology.Workers.Count == 0 {
		p.Log.Info("workers: no workers configured, skipping")
		return nil
	}

	p.Log.Info("workers: starting", "count", cfg.Topology.Workers.Count)

	terraformDir := filepath.Join(opts.ProjectRoot, "infrastructure", "terraform", "environments", opts.TerraformEnv)

	tf := terraform.New(
		terraformDir,
		terraform.WithLogger(p.Log),
		terraform.WithEnv(p.Exec.SnapshotEnv()),
	)
	defer tf.ZeroizeEnv()

	if err := tf.Init(ctx); err != nil {
		return tf.WithLockHint(&errtypes.ClusterError{Msg: "terraform init failed", Err: err})
	}

	applyOpts := terraform.ApplyOptions{
		AutoApprove: true,
		Vars: map[string]string{
			"start_workers_immediately": "true",
		},
		Targets: []string{"module.okd_cluster.proxmox_virtual_environment_vm.worker"},
	}

	snapPath, snapErr := tf.SnapshotState(ctx)
	if snapErr != nil {
		return &errtypes.ClusterError{Msg: "workers: state snapshot failed", Err: snapErr}
	}

	if err := tf.Apply(ctx, applyOpts); err != nil {
		msg := "failed to start worker VMs"
		if snapPath != "" {
			msg = fmt.Sprintf("failed to start worker VMs (state backup: %s)", snapPath)
		}
		return tf.WithLockHint(&errtypes.ClusterError{Msg: msg, Err: err})
	}

	p.Log.Info("workers: all worker nodes started")
	return nil
}

// workersAlreadyRunning returns true when cfg.Topology.Workers.Count or more
// worker nodes are registered in the cluster. A cluster-unreachable error
// returns false so StartWorkerVMs runs as the safe fallback.
func (p *Phase) workersAlreadyRunning(ctx context.Context, cfg *config.Config) (bool, error) {
	if cfg.Topology.Workers.Count == 0 {
		return true, nil
	}
	out, err := p.OcOutput(ctx, "get", "nodes",
		"-l", "node-role.kubernetes.io/worker",
		"--no-headers", "--ignore-not-found")
	if err != nil {
		return false, nil //nolint:nilerr // cluster-unreachable is non-fatal: report not-done so StartWorkerVMs runs as the safe fallback
	}
	count := 0
	for line := range strings.Lines(out) {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count >= cfg.Topology.Workers.Count, nil
}
