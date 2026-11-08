package proxmox

import (
	"net/url"
	"reflect"
	"testing"
)

func TestVMSpecValues(t *testing.T) {
	spec := VMSpec{
		VMID:      105,
		Name:      "grappleberry-worker-0",
		CPUType:   "x86-64-v3",
		Cores:     4,
		MemoryMiB: 16384,
		Disks: []DiskSpec{
			{Slot: "scsi0", Storage: "local-lvm", SizeGiB: 120, Serial: "grappleberry-worker-0-root"},
			{Slot: "scsi1", Storage: "ceph-rbd", SizeGiB: 500, Serial: "grappleberry-worker-0-data"},
		},
		EFIStorage: "local-lvm",
		MAC:        "BC:24:11:AA:BB:CC",
		Bridge:     "vmbr0",
		VLAN:       227,
		ISOVolID:   "local:iso/grappleberry-worker.iso",
		Tag:        "grappleberry",
		Pool:       "okd",
	}
	want := url.Values{
		"vmid":     {"105"},
		"name":     {"grappleberry-worker-0"},
		"machine":  {"q35"},
		"bios":     {"ovmf"},
		"efidisk0": {"local-lvm:1,efitype=4m,pre-enrolled-keys=0"},
		"scsihw":   {"virtio-scsi-single"},
		"scsi0":    {"local-lvm:120,serial=grappleberry-worker-0-root,iothread=1,discard=on,cache=none,aio=io_uring"},
		"scsi1":    {"ceph-rbd:500,serial=grappleberry-worker-0-data,iothread=1,discard=on,cache=none,aio=io_uring"},
		"net0":     {"virtio=BC:24:11:AA:BB:CC,bridge=vmbr0,tag=227"},
		"ide2":     {"local:iso/grappleberry-worker.iso,media=cdrom"},
		"boot":     {"order=scsi0;ide2"},
		"cpu":      {"x86-64-v3"},
		"cores":    {"4"},
		"sockets":  {"1"},
		"memory":   {"16384"},
		"balloon":  {"0"},
		"agent":    {"enabled=1"},
		"tags":     {"grappleberry"},
		"onboot":   {"1"},
		"ostype":   {"l26"},
		"pool":     {"okd"},
	}
	if got := spec.values(); !reflect.DeepEqual(got, want) {
		t.Fatalf("values() =\n%v\nwant\n%v", got, want)
	}

	t.Run("no pool and untagged bridge", func(t *testing.T) {
		spec.Pool = ""
		spec.VLAN = 0
		got := spec.values()
		if _, ok := got["pool"]; ok {
			t.Errorf("values() has pool=%q with Pool unset", got.Get("pool"))
		}
		if net0 := got.Get("net0"); net0 != "virtio=BC:24:11:AA:BB:CC,bridge=vmbr0" {
			t.Errorf("net0 = %q, want no ,tag= with VLAN 0", net0)
		}
	})
}
