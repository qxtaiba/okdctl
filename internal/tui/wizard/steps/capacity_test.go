package steps

import "testing"

func TestWizardCapacitySnapshotOnlineTotalsSkipOffline(t *testing.T) {
	snapshot := &WizardCapacitySnapshot{discovery: &proxmoxDiscovery{Nodes: []proxmoxNode{
		{Name: "online", Status: "online", CPUs: 8, MemGB: 16},
		{Name: "offline", Status: "offline", CPUs: 64, MemGB: 256},
	}}}

	if got := snapshot.OnlineTotals(); got != (CapacityTotals{CPUs: 8, MemoryGB: 16, Known: true}) {
		t.Fatalf("OnlineTotals() = %+v, want the online node alone", got)
	}
	var none *WizardCapacitySnapshot
	if got := none.OnlineTotals(); got != (CapacityTotals{}) {
		t.Fatalf("nil snapshot OnlineTotals() = %+v, want zero", got)
	}
}
