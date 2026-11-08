package steps

import "testing"

func TestWizardCapacitySnapshotOnlineTotalsPreserveUnknownAndSkipOffline(t *testing.T) {
	snapshot := &WizardCapacitySnapshot{discovery: &proxmoxDiscovery{Nodes: []proxmoxNode{
		{Name: "online-known", Status: "online", CPUs: 0, CPUsKnown: true, MemGB: 0, MemKnown: true},
		{Name: "online-unknown", Status: "online", CPUs: 8, CPUsKnown: true},
		{Name: "offline", Status: "offline", CPUs: 64, CPUsKnown: true, MemGB: 256, MemKnown: true},
	}}}

	totals := snapshot.OnlineTotals()
	if totals.CPUs != 8 || !totals.CPUsKnown || totals.MemoryGB != 0 || totals.MemoryKnown {
		t.Fatalf("OnlineTotals() = %+v, want known CPU sum 8 and unknown memory", totals)
	}

	var none *WizardCapacitySnapshot
	if got := none.OnlineTotals(); got != (CapacityTotals{}) {
		t.Fatalf("nil snapshot OnlineTotals() = %+v, want zero", got)
	}
}
