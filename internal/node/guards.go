package node

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

// tfResourcePrefix is the terraform address prefix for the OKD VM resources,
// shared with destroy's allowlist. Node ops address a single count instance by
// index (…worker[N]).
const tfResourcePrefix = "module.okd_cluster.proxmox_virtual_environment_vm."

func workerAddress(index int) string {
	return fmt.Sprintf("%sworker[%d]", tfResourcePrefix, index)
}

func masterAddress(index int) string {
	return fmt.Sprintf("%smaster[%d]", tfResourcePrefix, index)
}

// validateWorkerRemovable enforces the count-index constraint: terraform count
// reduction only ever destroys the highest-index instance, so the requested
// node must be a worker whose trailing index equals workerCount-1 and is the
// maximum among the cluster's workers. Returns an actionable error otherwise.
func validateWorkerRemovable(nodes []cluster.NodeDetail, target string, workerCount int) error {
	var targetNode *cluster.NodeDetail
	var workers []cluster.NodeDetail
	for i := range nodes {
		if nodes[i].Role == nodetypes.RoleWorker {
			workers = append(workers, nodes[i])
		}
		if nodes[i].Name == target {
			targetNode = &nodes[i]
		}
	}
	if targetNode == nil {
		return fmt.Errorf("node %q not found in cluster; run 'okdctl node list' to list nodes", target)
	}
	if targetNode.Role != nodetypes.RoleWorker {
		return fmt.Errorf("node %q is a %s; only worker nodes can be removed (master add/remove is not supported)", target, targetNode.Role)
	}
	idx, ok := cluster.NodeIndex(target)
	if !ok {
		return fmt.Errorf("cannot derive a terraform index from node name %q; okdctl removes the highest-numbered worker (e.g. worker%d)", target, workerCount-1)
	}
	maxIdx := -1
	for _, w := range workers {
		if i, ok := cluster.NodeIndex(w.Name); ok && i > maxIdx {
			maxIdx = i
		}
	}
	if idx != maxIdx {
		return fmt.Errorf("node %q is index %d but the highest-numbered worker is index %d; remove workers top-down (terraform count reduction only removes the last instance)", target, idx, maxIdx)
	}
	if idx != workerCount-1 {
		return fmt.Errorf("node %q is index %d but topology worker_count is %d; expected the removable node to be index %d — reconcile config with the cluster before removing", target, idx, workerCount, workerCount-1)
	}
	return nil
}

// podNamesOnNode returns the namespace/name of every pod in pods placed on
// node, sorted. Callers pre-filter pods by label (OSD, router) so the placement
// filter here stays generic; a non-empty OSD result means removing the node
// destroys its CEPH-DATA disk and the data on it.
func podNamesOnNode(pods []cluster.PodPlacement, node string) []string {
	var names []string
	for _, p := range pods {
		if p.NodeName == node {
			names = append(names, p.Namespace+"/"+p.Name)
		}
	}
	sort.Strings(names)
	return names
}

// storageGuardVerdict decides whether removing node is permitted given the
// rook-ceph OSD pods scheduled on it. Shared by RemoveWorker's in-loop guard and
// compact's preflight so both refuse (or force-allow) identically. A non-empty
// osds with force=false returns a ConfigError; force=true warns and permits.
func storageGuardVerdict(node string, osds []string, force bool, log *slog.Logger) error {
	if len(osds) == 0 {
		return nil
	}
	if force {
		log.Warn("node: --force-storage set; removing a node with live OSDs destroys their CEPH-DATA disk", "node", node, "osds", len(osds))
		return nil
	}
	return &errtypes.ConfigError{Msg: fmt.Sprintf(
		"%s holds %d rook-ceph OSD(s) (%v); removing it destroys its CEPH-DATA disk and loses that data. Migrate OSDs off it first, or re-run with --force-storage.",
		node, len(osds), osds,
	)}
}

// projectCompactPeakMiB simulates compact's interleaved sequence (remove a
// worker, then grow the next master) and returns the peak host memory
// allocation reached. A freed worker always precedes a grown master, so the peak
// bounds the memory-budget guard's projection. growTargetMiB==0 models "no
// master grow" — the sequence only frees memory.
func projectCompactPeakMiB(allocatedMiB, workerMiB, masterCurMiB, growTargetMiB, numWorkers, numMasters int) int {
	masterDelta := 0
	if growTargetMiB > 0 {
		masterDelta = growTargetMiB - masterCurMiB
	}
	peak := allocatedMiB
	grows := 0
	for i := 0; i < numWorkers; i++ {
		allocatedMiB -= workerMiB
		if growTargetMiB > 0 && grows < numMasters {
			allocatedMiB += masterDelta
			grows++
		}
		if allocatedMiB > peak {
			peak = allocatedMiB
		}
	}
	for ; growTargetMiB > 0 && grows < numMasters; grows++ {
		allocatedMiB += masterDelta
		if allocatedMiB > peak {
			peak = allocatedMiB
		}
	}
	return peak
}

// ingressPodsOnWorkers returns router pods scheduled on any worker node. When
// non-empty and the control plane is not schedulable, draining the workers
// would strand ingress with nowhere to reschedule.
func ingressPodsOnWorkers(routerPods []cluster.PodPlacement, workers map[string]bool) []cluster.PodPlacement {
	var out []cluster.PodPlacement
	for _, p := range routerPods {
		if workers[p.NodeName] {
			out = append(out, p)
		}
	}
	return out
}

// validateMemoryBudget reports whether growing guest allocation by deltaMiB
// keeps the host within its physical RAM minus hostMemoryReserveMiB headroom.
// hostTotalMiB is the hypervisor's physical memory; allocatedMiB is the sum
// already committed to running guests. A non-positive delta always passes.
func validateMemoryBudget(hostTotalMiB, allocatedMiB, deltaMiB int) error {
	if deltaMiB <= 0 {
		return nil
	}
	projected := allocatedMiB + deltaMiB
	if projected+hostMemoryReserveMiB > hostTotalMiB {
		return fmt.Errorf("memory budget: growing by %d MiB would commit %d MiB of %d MiB host RAM (leaving < %d MiB reserve); free a node first",
			deltaMiB, projected, hostTotalMiB, hostMemoryReserveMiB)
	}
	return nil
}

// workerNameSet builds a lookup of worker node names for ingress placement checks.
func workerNameSet(nodes []cluster.NodeDetail) map[string]bool {
	set := make(map[string]bool)
	for _, n := range nodes {
		if n.Role == nodetypes.RoleWorker {
			set[n.Name] = true
		}
	}
	return set
}

func joinPodNames(pods []cluster.PodPlacement) string {
	names := make([]string, len(pods))
	for i, p := range pods {
		names[i] = p.Namespace + "/" + p.Name + "@" + p.NodeName
	}
	return strings.Join(names, ", ")
}
