package steps

import "github.com/qxtaiba/okdctl/internal/config"

// CapacityStorage describes one storage pool as Proxmox reported it.
type CapacityStorage struct {
	Name       string
	Content    string
	TotalGB    int
	TotalKnown bool
}

// CapacityBridge describes one discovered Proxmox bridge.
type CapacityBridge struct {
	Name string
	CIDR string
}

// CapacityNode retains the discovered capacity and inventory for one node.
type CapacityNode struct {
	Name         string
	Status       string
	CPUs         int
	CPUsKnown    bool
	MemoryGB     int
	MemoryKnown  bool
	Storage      []CapacityStorage
	StorageKnown bool
	Bridges      []CapacityBridge
	BridgesKnown bool
}

// CapacityTotals summarizes resources reported by online nodes.
// Known flags are false unless every online node reported that resource.
type CapacityTotals struct {
	CPUs        int
	MemoryGB    int
	CPUsKnown   bool
	MemoryKnown bool
}

// WizardCapacitySnapshot shares one discovery result across configure steps.
type WizardCapacitySnapshot struct {
	cfg       *config.Config
	discovery *proxmoxDiscovery
}

type onlineCapacity struct {
	CPUs      int
	MemoryGB  int
	CPUsKnown bool
	MemKnown  bool
}

// IsWizardStepState marks the snapshot as available through BuiltSteps.States.
func (s *WizardCapacitySnapshot) IsWizardStepState() {}

// Nodes returns deep copies of discovered nodes and their inventories.
func (s *WizardCapacitySnapshot) Nodes() []CapacityNode {
	if s == nil || s.discovery == nil {
		return nil
	}
	nodes := make([]CapacityNode, len(s.discovery.Nodes))
	for i := range s.discovery.Nodes {
		node := &s.discovery.Nodes[i]
		nodes[i] = CapacityNode{
			Name: node.Name, Status: node.Status, CPUs: node.CPUs, CPUsKnown: node.CPUsKnown,
			MemoryGB: node.MemGB, MemoryKnown: node.MemKnown,
			StorageKnown: node.StorageKnown, BridgesKnown: node.BridgesKnown,
		}
		nodes[i].Storage = make([]CapacityStorage, len(node.Storage))
		for j := range node.Storage {
			pool := node.Storage[j]
			nodes[i].Storage[j] = CapacityStorage(pool)
		}
		nodes[i].Bridges = make([]CapacityBridge, len(node.Bridges))
		for j := range node.Bridges {
			bridge := node.Bridges[j]
			nodes[i].Bridges[j] = CapacityBridge(bridge)
		}
	}
	return nodes
}

// OnlineTotals returns aggregate CPU and memory while preserving unknown data.
func (s *WizardCapacitySnapshot) OnlineTotals() CapacityTotals {
	var total onlineCapacity
	if s == nil || s.discovery == nil {
		return CapacityTotals{}
	}
	first := true
	for i := range s.discovery.Nodes {
		node := &s.discovery.Nodes[i]
		if node.Status != proxmoxStatusOnline {
			continue
		}
		if first {
			total.CPUsKnown = true
			total.MemKnown = true
			first = false
		}
		total.CPUs += node.CPUs
		total.MemoryGB += node.MemGB
		total.CPUsKnown = total.CPUsKnown && node.CPUsKnown
		total.MemKnown = total.MemKnown && node.MemKnown
	}
	return CapacityTotals{
		CPUs: total.CPUs, MemoryGB: total.MemoryGB,
		CPUsKnown: total.CPUsKnown, MemoryKnown: total.MemKnown,
	}
}

func (s *WizardCapacitySnapshot) counts() (controlPlanes, workers int, ok bool) {
	if s == nil || s.cfg == nil {
		return 0, 0, false
	}
	return s.cfg.Topology.ControlPlane.Count, s.cfg.Topology.Workers.Count, true
}

// EffectiveResourceTotals is the one vcpu/memory/disk total the resources
// and review screens both render from.
type EffectiveResourceTotals struct {
	CPU        int
	MemoryMB   int
	OSDiskGB   int
	DataDiskGB int
}

// ComputeEffectiveResourceTotals derives total vcpu, memory, os-disk, and
// data-disk allocation across bootstrap, control-plane, and worker nodes,
// resolving omitted sizing through config.NormalizeTopology as a deploy does.
func ComputeEffectiveResourceTotals(topology *config.TopologyConfig, disks *config.DisksConfig) EffectiveResourceTotals {
	t := config.NormalizeTopology(topology)
	cp, workers := t.ControlPlane, t.Workers
	return EffectiveResourceTotals{
		CPU:      cp.CPU*cp.Count + workers.CPU*workers.Count + t.Bootstrap.CPU,
		MemoryMB: cp.MemoryMB*cp.Count + workers.MemoryMB*workers.Count + t.Bootstrap.MemoryMB,
		// The bootstrap vm always takes the control-plane os disk size, whatever Bootstrap.DiskGB holds.
		OSDiskGB:   cp.DiskGB*cp.Count + workers.DiskGB*workers.Count + cp.DiskGB,
		DataDiskGB: disks.WorkerDataSizeGB*workers.Count + disks.ControlPlaneDataSizeGB*cp.Count,
	}
}
