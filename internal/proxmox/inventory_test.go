package proxmox

import (
	"errors"
	"net/http"
	"reflect"
	"slices"
	"testing"
)

func TestVersion(t *testing.T) {
	d := newFakeAPI(t, nil).driver(t)
	got, err := d.Version(t.Context())
	if err != nil {
		t.Fatalf("Version(): %v", err)
	}
	want := Version{Version: "9.0.10", Release: "9.0"}
	if got != want {
		t.Fatalf("Version() = %+v, want %+v", got, want)
	}
}

func TestNodes(t *testing.T) {
	d := newFakeAPI(t, nil).driver(t)
	got, err := d.Nodes(t.Context())
	if err != nil {
		t.Fatalf("Nodes(): %v", err)
	}
	want := []NodeInfo{{
		Name:        "pve",
		Status:      "online",
		MemTotalMiB: 65536,
		MemUsedMiB:  20480,
		CPUs:        32,
		CPUModel:    "AMD Ryzen 9 7950X 16-Core Processor",
		CPUFlags:    []string{"fpu", "vme", "de", "pse", "sse", "sse2", "ssse3", "sse4_1", "sse4_2", "avx", "avx2", "x86-64-v3"},
		PVEVersion:  "pve-manager/9.0.10/abcdef0123456789",
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Nodes() = %+v, want %+v", got, want)
	}
}

func TestNodesSkipsStatusOfOfflineNode(t *testing.T) {
	f := newFakeAPI(t, map[string]http.HandlerFunc{
		"/nodes": jsonHandler(http.StatusOK, `{"data":[
			{"node":"pve","status":"online","maxmem":68719476736,"mem":21474836480,"maxcpu":16},
			{"node":"pve2","status":"offline","maxmem":34359738368,"mem":0,"maxcpu":8}]}`),
	})
	got, err := f.driver(t).Nodes(t.Context())
	if err != nil {
		t.Fatalf("Nodes(): %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Nodes() returned %d nodes, want 2: %+v", len(got), got)
	}
	want := NodeInfo{Name: "pve2", Status: "offline", MemTotalMiB: 32768, CPUs: 8}
	if !reflect.DeepEqual(got[1], want) {
		t.Fatalf("offline node = %+v, want %+v", got[1], want)
	}
	if f.called("GET /nodes/pve2/status") {
		t.Fatal("Nodes() queried /nodes/pve2/status for an offline node")
	}
	if !f.called("GET /nodes/pve/status") {
		t.Fatal("Nodes() did not query /nodes/pve/status for the online node")
	}
}

func TestStorages(t *testing.T) {
	d := newFakeAPI(t, nil).driver(t)
	got, err := d.Storages(t.Context(), "pve")
	if err != nil {
		t.Fatalf("Storages(): %v", err)
	}
	want := []StorageInfo{
		{Name: "local", Type: "dir", Active: true, Content: []string{"iso", "vztmpl"}, AvailBytes: 85899345920, TotalBytes: 107374182400},
		{Name: "local-lvm", Type: "lvmthin", Active: true, Content: []string{"images", "rootdir"}, AvailBytes: 429496729600, TotalBytes: 536870912000},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Storages() = %+v, want %+v", got, want)
	}
}

func TestStoragesSharedFlag(t *testing.T) {
	f := newFakeAPI(t, map[string]http.HandlerFunc{
		"/nodes/pve/storage": jsonHandler(http.StatusOK, `{"data":[
			{"storage":"cephfs","type":"cephfs","shared":1,"active":1,"enabled":1,"content":"iso","avail":1,"total":2}]}`),
	})
	got, err := f.driver(t).Storages(t.Context(), "pve")
	if err != nil {
		t.Fatalf("Storages(): %v", err)
	}
	if len(got) != 1 || !got[0].Shared {
		t.Fatalf("Storages() = %+v, want one shared storage", got)
	}
}

func TestBridges(t *testing.T) {
	f := newFakeAPI(t, nil)
	got, err := f.driver(t).Bridges(t.Context(), "pve")
	if err != nil {
		t.Fatalf("Bridges(): %v", err)
	}
	want := []Bridge{{Name: "vmbr0", VLANAware: true, Active: true}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Bridges() = %+v, want %+v", got, want)
	}
	if !f.called("GET /nodes/pve/network?type=any_bridge") {
		t.Fatal("Bridges() did not filter the network listing with type=any_bridge")
	}
}

func TestListVMs(t *testing.T) {
	d := newFakeAPI(t, nil).driver(t)
	got, err := d.ListVMs(t.Context())
	if err != nil {
		t.Fatalf("ListVMs(): %v", err)
	}
	want := []VMSummary{
		{VMRef: VMRef{Node: "pve", VMID: 100}, Name: "grappleberry-bootstrap", Status: "running", Tags: []string{"grappleberry", "okd"}, Pool: "okd", MaxMemMiB: 16384, MaxCPU: 4},
		{VMRef: VMRef{Node: "pve", VMID: 101}, Name: "grappleberry-master-0", Status: "running", Tags: []string{"grappleberry", "okd"}, Pool: "okd", MaxMemMiB: 16384, MaxCPU: 4},
		{VMRef: VMRef{Node: "pve", VMID: 200}, Name: "lab-9f3a1c-worker-0", Status: "stopped", Tags: []string{"lab-9f3a1c"}, MaxMemMiB: 8192, MaxCPU: 2},
		{VMRef: VMRef{Node: "pve", VMID: 201}, Name: "lab-9f3a1cx-worker-0", Status: "stopped", Tags: []string{"lab-9f3a1cx"}, MaxMemMiB: 8192, MaxCPU: 2},
		{VMRef: VMRef{Node: "pve", VMID: 9000}, Name: "fcos-template", Status: "stopped", MaxMemMiB: 2048, MaxCPU: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListVMs() = %+v, want %+v", got, want)
	}

	t.Run("whole tag match", func(t *testing.T) {
		var matched []int
		for _, vm := range got {
			if slices.Contains(vm.Tags, "lab-9f3a1c") {
				matched = append(matched, vm.VMID)
			}
		}
		if !slices.Equal(matched, []int{200}) {
			t.Fatalf("VMs tagged lab-9f3a1c = %v, want [200] only (lab-9f3a1cx must not match)", matched)
		}
	})
}

func TestPermissions(t *testing.T) {
	d := newFakeAPI(t, nil).driver(t)
	got, err := d.Permissions(t.Context())
	if err != nil {
		t.Fatalf("Permissions(): %v", err)
	}
	want := Permissions{
		"/":              {"Sys.Audit": true},
		"/nodes/pve":     {"Sys.Audit": true, "Sys.Console": true},
		"/storage/local": {"Datastore.AllocateSpace": true, "Datastore.AllocateTemplate": true, "Datastore.Audit": true},
		"/vms":           {"VM.Allocate": true, "VM.Audit": true, "VM.Config.Disk": true, "VM.PowerMgmt": true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Permissions() = %v, want %v", got, want)
	}
}

func TestInventoryMapsForbiddenToErrPermission(t *testing.T) {
	f := newFakeAPI(t, map[string]http.HandlerFunc{
		"/version":                   jsonHandler(http.StatusForbidden, `{"data":null}`),
		"/cluster/resources?type=vm": jsonHandler(http.StatusForbidden, `{"data":null}`),
	})
	d := f.driver(t)
	if _, err := d.Version(t.Context()); !errors.Is(err, ErrPermission) {
		t.Fatalf("Version() error = %v, want ErrPermission", err)
	}
	if _, err := d.ListVMs(t.Context()); !errors.Is(err, ErrPermission) {
		t.Fatalf("ListVMs() error = %v, want ErrPermission", err)
	}
}
