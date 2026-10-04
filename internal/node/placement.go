package node

import (
	"context"
	"fmt"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

// HostCapacity is the observed budget for one Proxmox node and OS datastore.
type HostCapacity struct {
	TotalMiB, AllocatedMiB, AvailableDiskGB int
	Shared                                  bool
}

type vmOwner interface {
	VMOwner(context.Context, int) (string, error)
}

func (r *Runner) targetHosts(ctx context.Context, role nodetypes.NodeRole, names []string, existing bool) (map[string]int, error) {
	hosts := make(map[string]int)
	for _, name := range names {
		index, ok := cluster.NodeIndex(name)
		if !ok {
			return nil, &errtypes.ConfigError{Msg: fmt.Sprintf("cannot resolve placement for %s", name)}
		}
		host, vmid := r.vmTarget(role, index)
		if observer, ok := r.Power.(vmOwner); ok && existing {
			observed, err := observer.VMOwner(ctx, vmid)
			if err != nil {
				return nil, fmt.Errorf("resolve %s placement: %w", name, err)
			}
			host = observed
		}
		if host == "" && r.Capacity != nil {
			return nil, fmt.Errorf("proxmox host for %s is unknown", name)
		}
		hosts[host]++
	}
	return hosts, nil
}

func (r *Runner) checkPlacementCapacity(ctx context.Context, hosts map[string]int, memoryDelta, diskDelta int) error {
	storage := ""
	if r.Cfg.Provider.Proxmox != nil {
		storage = r.Cfg.Provider.Proxmox.Storage
	}
	return r.checkDatastoreCapacity(ctx, hosts, storage, memoryDelta, diskDelta)
}

func (r *Runner) checkDatastoreCapacity(ctx context.Context, hosts map[string]int, storage string, memoryDelta, diskDelta int) error {
	if r.Capacity == nil {
		if len(hosts) > 1 && (memoryDelta > 0 || diskDelta > 0) {
			return &errtypes.ConfigError{Msg: "multi-host capacity requires a Proxmox probe for each destination"}
		}
		return nil
	}
	if memoryDelta <= 0 && diskDelta <= 0 {
		return nil
	}
	totalCount := 0
	for _, count := range hosts {
		totalCount += count
	}
	for host, count := range hosts {
		capacity, err := r.Capacity(ctx, host, storage)
		if err != nil {
			return fmt.Errorf("probe capacity on %s/%s: %w", host, storage, err)
		}
		if err := validateMemoryBudget(capacity.TotalMiB, capacity.AllocatedMiB, memoryDelta*count); err != nil {
			return fmt.Errorf("host %s: %w", host, err)
		}
		if diskDelta > 0 {
			diskCount := count
			if capacity.Shared {
				diskCount = totalCount
			}
			if err := validateDatastoreBudget(capacity.AvailableDiskGB, diskDelta*diskCount); err != nil {
				return fmt.Errorf("host %s datastore %s: %w", host, storage, err)
			}
		}
	}
	return nil
}

func (r *Runner) projectCompactPlacement(ctx context.Context, workers, masters []string, opts CompactOptions) error {
	masterHosts, err := r.targetHosts(ctx, nodetypes.RoleMaster, masters, true)
	if err != nil {
		return err
	}
	workerHosts, err := r.targetHosts(ctx, nodetypes.RoleWorker, workers, true)
	if err != nil {
		return err
	}
	if opts.GrowMasterMemoryMB <= 0 {
		return nil
	}
	if r.Capacity == nil {
		for host := range workerHosts {
			if _, ok := masterHosts[host]; !ok {
				masterHosts[host] = 0
			}
		}
		if len(masterHosts) > 1 {
			return &errtypes.ConfigError{Msg: "multi-host compaction requires a capacity probe for each destination"}
		}
		return r.projectCompactMemory(len(workers), len(masters), opts)
	}
	// Reserve all master growth up front; worker removal on another host frees no local memory.
	delta := max(0, opts.GrowMasterMemoryMB-r.Cfg.Topology.ControlPlane.MemoryMB)
	return r.checkPlacementCapacity(ctx, masterHosts, delta, 0)
}

func (r *Runner) checkAddCapacity(ctx context.Context, hosts map[string]int) error {
	px := r.Cfg.Provider.Proxmox
	osDisk, dataDisk := r.Cfg.Topology.Workers.DiskGB, r.Cfg.Disks.WorkerDataSizeGB
	if px.DataStorage == px.Storage {
		osDisk += dataDisk
	}
	if err := r.checkPlacementCapacity(ctx, hosts, r.Cfg.Topology.Workers.MemoryMB, osDisk); err != nil {
		return err
	}
	if px.DataStorage != px.Storage && dataDisk > 0 {
		return r.checkDatastoreCapacity(ctx, hosts, px.DataStorage, 0, dataDisk)
	}
	return nil
}
