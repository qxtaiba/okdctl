package node

import (
	"context"
	"fmt"
	"sort"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/templates"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

// CompactOptions tunes cluster compaction.
type CompactOptions struct {
	// IngressReplicas sets the compact IngressController replica count (masters
	// serve ingress once workers are gone).
	IngressReplicas int
	// GrowMasterMemoryMB, when > 0, resizes each master after the workers are
	// removed, interleaved so growth never precedes freeing a worker.
	GrowMasterMemoryMB int
	ForceStorage       bool
	// Host memory budget, from a read-only Proxmox probe; zero skips the check.
	HostTotalMiB     int
	HostAllocatedMiB int
}

// Compact consolidates the cluster onto its control plane: make masters
// schedulable, apply the compact IngressController, then remove workers
// top-down — interleaving an optional master grow after each removal so the
// memory budget is respected (a freed worker precedes a grown master). It
// composes RemoveWorker and Resize; it adds no new mutation mechanics.
func (r *Runner) Compact(ctx context.Context, opts CompactOptions) error {
	if err := r.waitEtcdHealthy(ctx, "compact-preflight"); err != nil {
		return err
	}

	nodes, err := r.Cluster.ListNodes(ctx)
	if err != nil {
		return &errtypes.ClusterError{Msg: "list nodes", Err: err}
	}
	workers := workersByIndexDesc(nodes)
	masters := mastersByIndexAsc(nodes)

	if r.DryRun {
		r.Log.Info("node: dry-run — compact plan",
			"workers_to_remove", len(workers), "masters", len(masters),
			"grow_master_mb", opts.GrowMasterMemoryMB)
		return nil
	}

	if err := r.enableSchedulableAndIngress(ctx, opts.IngressReplicas); err != nil {
		return err
	}

	masterGrows := 0
	for i, w := range workers {
		if err := r.RemoveWorker(ctx, w, RemoveOptions{ForceStorage: opts.ForceStorage, DrainTimeout: "10m"}); err != nil {
			return fmt.Errorf("compact: remove worker %s: %w", w, err)
		}
		// Interleave: after freeing a worker, grow the next master so allocation
		// never peaks above the pre-compaction commitment.
		if opts.GrowMasterMemoryMB > 0 && masterGrows < len(masters) && i < len(masters) {
			m := masters[masterGrows]
			if err := r.Resize(ctx, ResizeScope{Node: m}, ResizeOptions{
				MemoryMB:         opts.GrowMasterMemoryMB,
				HostTotalMiB:     opts.HostTotalMiB,
				HostAllocatedMiB: opts.HostAllocatedMiB,
			}); err != nil {
				return fmt.Errorf("compact: grow master %s: %w", m, err)
			}
			masterGrows++
		}
	}

	// Grow any remaining masters once all workers are gone.
	for ; opts.GrowMasterMemoryMB > 0 && masterGrows < len(masters); masterGrows++ {
		m := masters[masterGrows]
		if err := r.Resize(ctx, ResizeScope{Node: m}, ResizeOptions{
			MemoryMB:         opts.GrowMasterMemoryMB,
			HostTotalMiB:     opts.HostTotalMiB,
			HostAllocatedMiB: opts.HostAllocatedMiB,
		}); err != nil {
			return fmt.Errorf("compact: grow master %s: %w", m, err)
		}
	}

	if err := r.waitEtcdHealthy(ctx, "compact-final"); err != nil {
		return err
	}
	r.Log.Info("node: compaction complete", "masters", len(masters))
	return nil
}

func (r *Runner) enableSchedulableAndIngress(ctx context.Context, replicas int) error {
	if err := r.Cluster.SetMastersSchedulable(ctx, true); err != nil {
		return err
	}
	if replicas <= 0 {
		replicas = 2
	}
	manifest, err := templates.RenderCompactIngress(templates.CompactIngressData{Replicas: replicas})
	if err != nil {
		return &errtypes.ClusterError{Msg: "render compact ingress", Err: err}
	}
	if err := r.Cluster.Apply(ctx, []byte(manifest)); err != nil {
		return &errtypes.ClusterError{Msg: "apply compact ingress controller", Err: err}
	}
	r.Log.Info("node: control plane schedulable and compact ingress applied", "ingress_replicas", replicas)
	return nil
}

func workersByIndexDesc(nodes []cluster.NodeDetail) []string {
	return namesByIndex(nodes, nodetypes.RoleWorker, false)
}

func mastersByIndexAsc(nodes []cluster.NodeDetail) []string {
	return namesByIndex(nodes, nodetypes.RoleMaster, true)
}

func namesByIndex(nodes []cluster.NodeDetail, role nodetypes.NodeRole, ascending bool) []string {
	type ni struct {
		name string
		idx  int
	}
	var items []ni
	for _, n := range nodes {
		if n.Role != role {
			continue
		}
		idx, ok := cluster.NodeIndex(n.Name)
		if !ok {
			continue
		}
		items = append(items, ni{name: n.Name, idx: idx})
	}
	sort.Slice(items, func(i, j int) bool {
		if ascending {
			return items[i].idx < items[j].idx
		}
		return items[i].idx > items[j].idx
	})
	names := make([]string, len(items))
	for i, it := range items {
		names[i] = it.name
	}
	return names
}
