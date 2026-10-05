package steps

import "testing"

func TestWizardCapacitySnapshotPreservesUnknownAndExcludesOfflineTotals(t *testing.T) {
	snapshot := &WizardCapacitySnapshot{discovery: &proxmoxDiscovery{Nodes: []proxmoxNode{
		{Name: "online-known", Status: "online", CPUs: 0, CPUsKnown: true, MemGB: 0, MemKnown: true},
		{Name: "online-unknown", Status: "online", CPUs: 8, CPUsKnown: true},
		{Name: "offline", Status: "offline", CPUs: 64, CPUsKnown: true, MemGB: 256, MemKnown: true},
	}}}

	totals := snapshot.OnlineTotals()
	if totals.CPUs != 8 || !totals.CPUsKnown || totals.MemoryGB != 0 || totals.MemoryKnown {
		t.Fatalf("OnlineTotals() = %+v, want known CPU sum 8 and unknown memory", totals)
	}
	nodes := snapshot.Nodes()
	if len(nodes) != 3 || !nodes[0].CPUsKnown || !nodes[0].MemoryKnown || nodes[1].MemoryKnown {
		t.Fatalf("Nodes() did not preserve per-node known flags: %+v", nodes)
	}
	nodes[0].CPUs = 99
	if got := snapshot.Nodes()[0].CPUs; got != 0 {
		t.Fatalf("Nodes() exposes mutable snapshot storage: CPU = %d", got)
	}
}
