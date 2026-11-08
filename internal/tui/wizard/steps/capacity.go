package steps

import "github.com/qxtaiba/okdctl/internal/config"

// CapacityTotals sums the online nodes' cpu and memory; a Known flag is
// false unless every online node reported that resource.
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

// IsWizardStepState marks the snapshot as available through BuiltSteps.States.
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
