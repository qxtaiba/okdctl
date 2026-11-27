package steps

import (
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/model"
)

// CapacityTotals sums the online nodes' cpu and memory; a Known flag is false
// unless every online node reported that resource.
type CapacityTotals struct {
	CPUs        int
	MemoryGB    int
	CPUsKnown   bool
	MemoryKnown bool
}

// WizardCapacitySnapshot shares the session config and one discovery result
// across the configure steps.
type WizardCapacitySnapshot struct {
	cfg       *config.Config
	discovery *proxmoxDiscovery
}

// IsWizardStepState marks the snapshot as a wizard.StepState.
func (s *WizardCapacitySnapshot) IsWizardStepState() {}

// OnlineTotals aggregates the online nodes' cpu and memory.
func (s *WizardCapacitySnapshot) OnlineTotals() CapacityTotals {
	var total CapacityTotals
	if s == nil || s.discovery == nil {
		return total
	}
	first := true
	for i := range s.discovery.Nodes {
		node := &s.discovery.Nodes[i]
		if node.Status != proxmoxStatusOnline {
			continue
		}
		if first {
			total.CPUsKnown, total.MemoryKnown = true, true
			first = false
		}
		total.CPUs += node.CPUs
		total.MemoryGB += node.MemGB
		total.CPUsKnown = total.CPUsKnown && node.CPUsKnown
		total.MemoryKnown = total.MemoryKnown && node.MemKnown
	}
	return total
}

func (s *WizardCapacitySnapshot) nodes() []proxmoxNode {
	if s == nil || s.discovery == nil {
		return nil
	}
	return s.discovery.Nodes
}

type demand struct{ cpu, memoryMiB, diskGiB int }

func (d *demand) add(n *model.NodeSpec) {
	d.cpu += n.CPU
	d.memoryMiB += n.MemoryMiB
	for _, disk := range n.Disks {
		d.diskGiB += disk.SizeGiB
	}
}

// demandByHost groups nodes by hostOf, or by each node's own host when nil.
func demandByHost(nodes []model.NodeSpec, hostOf func(*model.NodeSpec) string) map[string]*demand {
	use := make(map[string]*demand)
	for i := range nodes {
		n := &nodes[i]
		host := n.Host
		if hostOf != nil {
			host = hostOf(n)
		}
		d := use[host]
		if d == nil {
			d = &demand{}
			use[host] = d
		}
		d.add(n)
	}
	return use
}

func inventoryDemand(nodes []model.NodeSpec) demand {
	var d demand
	for i := range nodes {
		d.add(&nodes[i])
	}
	return d
}

func (n *proxmoxNode) exceeds(d *demand) bool {
	return (n.CPUsKnown && d.cpu > n.CPUs) || (n.MemKnown && d.memoryMiB > n.MemGB*1024)
}

func (t CapacityTotals) overCPU(d *demand) bool { return t.CPUsKnown && d.cpu > t.CPUs }
func (t CapacityTotals) overMemory(d *demand) bool {
	return t.MemoryKnown && d.memoryMiB > t.MemoryGB*1024
}
