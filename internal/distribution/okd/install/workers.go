package install

import (
	"context"
	"errors"
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
		terraform.WithEnv(p.Exec.Env),
	)
	defer tf.ZeroizeEnv()

	if err := tf.Init(ctx); err != nil {
		if hint := tf.LockHint(); hint != nil {
			return errors.Join(hint, &errtypes.ClusterError{Msg: "terraform init failed", Err: err})
		}
		return &errtypes.ClusterError{Msg: "terraform init failed", Err: err}
	}

	applyOpts := terraform.ApplyOptions{
		AutoApprove: true,
		Vars: map[string]string{
			"start_workers_immediately": "true",
		},
		// Without scoping, this apply reconciles the full state — a stray
		// tfvars edit elsewhere would be silently applied alongside the
		// worker start. Bootstrap takes the same precaution.
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
		wrapped := &errtypes.ClusterError{Msg: msg, Err: err}
		if hint := tf.LockHint(); hint != nil {
			return errors.Join(hint, wrapped)
		}
		return wrapped
	}

	p.Log.Info("workers: all worker nodes started successfully")
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
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count >= cfg.Topology.Workers.Count, nil
}
