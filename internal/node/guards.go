package node

import (
	"fmt"
	"sort"
	"strings"

	"github.com/qxtaiba/okdctl/internal/cluster"
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
		return fmt.Errorf("node %q not found in cluster; run 'okdctl status' to list nodes", target)
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

// osdPodsOnNode returns the names of rook-ceph OSD pods scheduled on node.
// A non-empty result means removing the node destroys its CEPH-DATA disk and
// the OSD data on it. Detection is generic: any pod matching the OSD label,
// in any namespace, placed on the target node.
func osdPodsOnNode(pods []cluster.PodPlacement, node string) []string {
	var names []string
	for _, p := range pods {
		if p.NodeName == node {
			names = append(names, p.Namespace+"/"+p.Name)
		}
	}
	sort.Strings(names)
	return names
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
