package node

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

// tfResourcePrefix is the terraform address prefix for OKD VM resources, shared
// with destroy's allowlist.
const tfResourcePrefix = "module.okd_cluster.proxmox_virtual_environment_vm."

func workerAddress(index int) string {
	return fmt.Sprintf("%sworker[%d]", tfResourcePrefix, index)
}

func masterAddress(index int) string {
	return fmt.Sprintf("%smaster[%d]", tfResourcePrefix, index)
}

// validateWorkerRemovable enforces the count-index rule: only the highest-index
// worker may be removed, since terraform count reduction only destroys the last
// instance.
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
		return &errtypes.ConfigError{Msg: fmt.Sprintf("node %q not found in cluster; run 'okdctl node list' to list nodes", target)}
	}
	if targetNode.Role != nodetypes.RoleWorker {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("node %q is a %s; only worker nodes can be removed (master add/remove is not supported)", target, targetNode.Role)}
	}
	idx, ok := cluster.NodeIndex(target)
	if !ok {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("cannot derive a terraform index from node name %q; okdctl removes the highest-numbered worker (e.g. worker%d)", target, workerCount-1)}
	}
	maxIdx := -1
	for _, w := range workers {
		if i, ok := cluster.NodeIndex(w.Name); ok {
			maxIdx = max(maxIdx, i)
		}
	}
	if idx != maxIdx {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("node %q is index %d but the highest-numbered worker is index %d; remove workers top-down (terraform count reduction only removes the last instance)", target, idx, maxIdx)}
	}
	if idx != workerCount-1 {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("node %q is index %d but topology worker_count is %d; expected the removable node to be index %d — reconcile config with the cluster before removing", target, idx, workerCount, workerCount-1)}
	}
	return nil
}

// podNamesOnNode returns sorted namespace/name pairs for pods on node; callers
// pre-filter by label (OSD, router).
func podNamesOnNode(pods []cluster.PodPlacement, node string) []string {
	var names []string
	for _, p := range pods {
		if p.NodeName == node {
			names = append(names, p.Namespace+"/"+p.Name)
		}
	}
	slices.Sort(names)
	return names
}

// storageGuardVerdict refuses removal when node has OSD pods unless force is
// set, which warns and permits instead.
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

// projectCompactPeakMiB simulates compact's remove-then-grow sequence and
// returns peak host memory; growTargetMiB==0 models a grow-free compact.
func projectCompactPeakMiB(allocatedMiB, workerMiB, masterCurMiB, growTargetMiB, numWorkers, numMasters int) int {
	masterDelta := 0
	if growTargetMiB > 0 {
		masterDelta = growTargetMiB - masterCurMiB
	}
	peak := allocatedMiB
	grows := 0
	for range numWorkers {
		allocatedMiB -= workerMiB
		if growTargetMiB > 0 && grows < numMasters {
			allocatedMiB += masterDelta
			grows++
		}
		peak = max(peak, allocatedMiB)
	}
	for ; growTargetMiB > 0 && grows < numMasters; grows++ {
		allocatedMiB += masterDelta
		peak = max(peak, allocatedMiB)
	}
	return peak
}

// ingressPodsOnWorkers returns router pods on any worker; non-empty with an
// unschedulable control plane means draining would strand ingress.
func ingressPodsOnWorkers(routerPods []cluster.PodPlacement, workers map[string]bool) []cluster.PodPlacement {
	var out []cluster.PodPlacement
	for _, p := range routerPods {
		if workers[p.NodeName] {
			out = append(out, p)
		}
	}
	return out
}

// validateMemoryBudget reports whether growing allocation by deltaMiB stays
// within hostTotalMiB minus reserve headroom; a non-positive delta always
// passes.
func validateMemoryBudget(hostTotalMiB, allocatedMiB, deltaMiB int) error {
	if deltaMiB <= 0 {
		return nil
	}
	projected := allocatedMiB + deltaMiB
	if projected+hostMemoryReserveMiB > hostTotalMiB {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("memory budget: growing by %d MiB would commit %d MiB of %d MiB host RAM (leaving < %d MiB reserve); free a node first",
			deltaMiB, projected, hostTotalMiB, hostMemoryReserveMiB)}
	}
	return nil
}

// validateDatastoreBudget refuses a grow the os datastore can never honor:
// thin provisioning defers the allocation, but discovering the shortfall at
// write time surfaces as guest I/O errors instead of a clean refusal.
func validateDatastoreBudget(availGB, deltaGB int) error {
	if deltaGB <= 0 {
		return nil
	}
	if deltaGB > availGB {
		return fmt.Errorf("datastore budget: growing os disks by %d GiB total exceeds the %d GiB available on the os datastore; free space or grow the pool first", deltaGB, availGB)
	}
	return nil
}

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
