package proxmox

import (
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func spikeRaw() map[string]any {
	return map[string]any{
		"digest":   "a1b2c3d4e5f60718293a4b5c6d7e8f9012345678",
		"name":     "spike-agent",
		"cores":    float64(4),
		"sockets":  float64(1),
		"cpu":      "host",
		"memory":   float64(30720),
		"balloon":  float64(0),
		"bios":     "ovmf",
		"machine":  "q35",
		"scsihw":   "virtio-scsi-single",
		"ostype":   "l26",
		"onboot":   float64(1),
		"agent":    "enabled=0,fstrim_cloned_disks=0,type=virtio",
		"boot":     "order=scsi0;ide2;net0",
		"tags":     "grappleberry;okd",
		"net0":     "virtio=02:d8:f7:00:23:32,bridge=vmbr0,firewall=0",
		"scsi0":    "local-lvm:vm-9010-disk-1,aio=io_uring,cache=none,discard=on,iothread=1,serial=OS-DISK,size=120G",
		"efidisk0": "local-lvm:vm-6010-disk-0,efitype=4m,pre-enrolled-keys=0,size=4M",
		"ide2":     "local:iso/spike-agent.iso,media=cdrom,size=904416K",
		"meta":     "creation-qemu=9.2.0,ctime=1762560000",
		"smbios1":  "uuid=3b5d8f1a-2c4e-4f6a-8b0c-1d3e5f7a9b2c",
		"vmgenid":  "9c1e3f5a-7b9d-4c2e-8f0a-2b4d6e8f0a1c",
		"hotplug":  "disk,network,usb",
	}
}

func spikeConfig() VMConfig {
	return VMConfig{
		Name: "spike-agent", Cores: 4, Sockets: 1, CPUType: "host", MemoryMiB: 30720,
		BIOS: "ovmf", Machine: "q35", SCSIHW: "virtio-scsi-single", OSType: "l26", OnBoot: true,
		BootOrder: []string{"scsi0", "ide2", "net0"}, Tags: []string{"grappleberry", "okd"},
		NICModel: "virtio", MAC: "02:d8:f7:00:23:32", Bridge: "vmbr0",
		Disks: map[string]DiskConfig{"scsi0": {
			Storage: "local-lvm", VolID: "local-lvm:vm-9010-disk-1", SizeGiB: 120, Serial: "OS-DISK",
			IOThread: true, Discard: true, Cache: "none", AIO: "io_uring",
		}},
		EFI:    &EFIConfig{Storage: "local-lvm", VolID: "local-lvm:vm-6010-disk-0", EFIType: "4m"},
		CDROM:  "local:iso/spike-agent.iso",
		Digest: "a1b2c3d4e5f60718293a4b5c6d7e8f9012345678",
	}
}

func mustParse(t *testing.T, raw map[string]any) VMConfig {
	t.Helper()
	c, err := ParseVMConfig(raw)
	if err != nil {
		t.Fatalf("ParseVMConfig(): %v", err)
	}
	return c
}

func TestParseVMConfig(t *testing.T) {
	t.Run("spike and master literals", func(t *testing.T) {
		if got, want := mustParse(t, spikeRaw()), spikeConfig(); !reflect.DeepEqual(got, want) {
			t.Fatalf("ParseVMConfig() =\n%+v\nwant\n%+v", got, want)
		}
	})

	t.Run("memory forms are equal", func(t *testing.T) {
		for _, v := range []any{float64(30720), "30720", "current=30720"} {
			raw := spikeRaw()
			raw["memory"] = v
			if got := mustParse(t, raw).MemoryMiB; got != 30720 {
				t.Errorf("memory %v: MemoryMiB = %d, want 30720", v, got)
			}
		}
	})

	t.Run("cpu forms", func(t *testing.T) {
		for in, want := range map[string]string{"host": "host", "x86-64-v3": "x86-64-v3", "cputype=x86-64-v3,flags=+aes": "x86-64-v3", "host,flags=+pdpe1gb": "host"} {
			raw := spikeRaw()
			raw["cpu"] = in
			if got := mustParse(t, raw).CPUType; got != want {
				t.Errorf("cpu %q: CPUType = %q, want %q", in, got, want)
			}
		}
	})

	t.Run("machine forms", func(t *testing.T) {
		for in, want := range map[string]string{"q35": "q35", "pc-q35-9.0": "q35", "type=q35": "q35", "pc-i440fx-9.0": "pc", "type=pc-q35-8.1,viommu=intel": "q35"} {
			raw := spikeRaw()
			raw["machine"] = in
			if got := mustParse(t, raw).Machine; got != want {
				t.Errorf("machine %q: Machine = %q, want %q", in, got, want)
			}
		}
	})

	t.Run("agent forms", func(t *testing.T) {
		for in, want := range map[any]bool{"1": true, float64(1): true, "enabled=1": true, "enabled=0,fstrim_cloned_disks=0,type=virtio": false, "0": false} {
			raw := spikeRaw()
			raw["agent"] = in
			if got := mustParse(t, raw).AgentEnabled; got != want {
				t.Errorf("agent %v: AgentEnabled = %v, want %v", in, got, want)
			}
		}
	})
}

func TestParseVMConfigDevices(t *testing.T) {
	t.Run("string numbers and bools", func(t *testing.T) {
		raw := spikeRaw()
		raw["cores"], raw["onboot"], raw["balloon"] = "8", "0", "2048"
		c := mustParse(t, raw)
		if c.Cores != 8 || c.OnBoot || c.Balloon != 2048 {
			t.Fatalf("Cores=%d OnBoot=%v Balloon=%d, want 8 false 2048", c.Cores, c.OnBoot, c.Balloon)
		}
	})

	t.Run("nic with vlan and macaddr alias", func(t *testing.T) {
		raw := spikeRaw()
		raw["net0"] = "model=e1000,macaddr=BC:24:11:AA:BB:CC,bridge=vmbr1,tag=42,firewall=1"
		c := mustParse(t, raw)
		if c.NICModel != "e1000" || c.MAC != "BC:24:11:AA:BB:CC" || c.Bridge != "vmbr1" || c.VLAN != 42 {
			t.Fatalf("nic = %q %q %q %d", c.NICModel, c.MAC, c.Bridge, c.VLAN)
		}
	})

	t.Run("cdrom none and absent", func(t *testing.T) {
		raw := spikeRaw()
		raw["ide2"] = "none,media=cdrom"
		if got := mustParse(t, raw).CDROM; got != "" {
			t.Errorf("CDROM = %q for none, want empty", got)
		}
		delete(raw, "ide2")
		if got := mustParse(t, raw).CDROM; got != "" {
			t.Errorf("CDROM = %q for absent ide2, want empty", got)
		}
	})

	t.Run("disk sizes", func(t *testing.T) {
		for in, want := range map[string]int{"32212254720": 30, "1T": 1024, "512M": 0, "120G": 120, "2048g": 2048} {
			raw := spikeRaw()
			raw["scsi1"] = "ceph-rbd:vm-9010-disk-2,size=" + in
			got := mustParse(t, raw).Disks["scsi1"]
			if got.SizeGiB != want || got.Storage != "ceph-rbd" || got.IOThread || got.Discard {
				t.Errorf("size %q: %+v, want SizeGiB %d on ceph-rbd with no flags", in, got, want)
			}
		}
	})

	t.Run("ide disks are disks but cdroms are not", func(t *testing.T) {
		raw := spikeRaw()
		raw["ide0"] = "local-lvm:vm-9010-disk-3,size=8G"
		raw["sata0"] = "local-lvm:vm-9010-disk-4,size=16G"
		c := mustParse(t, raw)
		if len(c.Disks) != 3 || c.Disks["ide0"].SizeGiB != 8 || c.Disks["sata0"].SizeGiB != 16 {
			t.Fatalf("Disks = %+v, want scsi0, ide0 and sata0", c.Disks)
		}
	})

	t.Run("unset keys stay zero", func(t *testing.T) {
		c := mustParse(t, map[string]any{"digest": "d"})
		if !reflect.DeepEqual(c, VMConfig{Digest: "d"}) {
			t.Fatalf("ParseVMConfig(digest only) = %+v", c)
		}
	})

	t.Run("malformed values are errors", func(t *testing.T) {
		for key, v := range map[string]any{"memory": "lots", "cores": "four", "scsi0": "local-lvm:vm-1-disk-0,size=big", "onboot": "yes", "net0": "virtio=aa,bridge=vmbr0,tag=x"} {
			raw := spikeRaw()
			raw[key] = v
			if _, err := ParseVMConfig(raw); err == nil || !strings.Contains(err.Error(), key) {
				t.Errorf("%s=%v: error = %v, want one naming %s", key, v, err, key)
			}
		}
	})
}

// pveStore synthesises what PVE writes back for a create form: numbers as
// JSON numbers, Proxmox-assigned volids and sizes, and the bookkeeping keys.
func pveStore(form url.Values) map[string]any {
	raw := map[string]any{
		"digest":  "0f0e0d0c0b0a09080706050403020100ffeeddcc",
		"meta":    "creation-qemu=9.2.0,ctime=1762560000",
		"smbios1": "uuid=3b5d8f1a-2c4e-4f6a-8b0c-1d3e5f7a9b2c",
		"vmgenid": "9c1e3f5a-7b9d-4c2e-8f0a-2b4d6e8f0a1c",
	}
	vmid := form.Get("vmid")
	for key := range form {
		v := form.Get(key)
		switch {
		case key == "vmid" || key == "pool":
		case key == "cores" || key == "sockets" || key == "memory" || key == "balloon" || key == "onboot":
			n, _ := strconv.Atoi(v)
			raw[key] = float64(n)
		case key == "efidisk0":
			storage, rest, _ := strings.Cut(v, ":")
			_, opts, _ := strings.Cut(rest, ",")
			raw[key] = storage + ":vm-" + vmid + "-disk-0," + opts + ",size=4M"
		case strings.HasPrefix(key, "scsi") && key != "scsihw":
			storage, rest, _ := strings.Cut(v, ":")
			size, opts, _ := strings.Cut(rest, ",")
			raw[key] = storage + ":vm-" + vmid + "-disk-" + key[4:] + "," + opts + ",size=" + size + "G"
		case key == "ide2":
			raw[key] = v + ",size=904416K"
		case key == "agent":
			raw[key] = strings.TrimPrefix(v, "enabled=")
		default:
			raw[key] = v
		}
	}
	return raw
}

func renderedConfig(t *testing.T, spec *VMSpec) VMConfig {
	t.Helper()
	return mustParse(t, pveStore(spec.values()))
}

func diffSpec() VMSpec {
	spec := newSpec(105, "grappleberry")
	spec.VLAN = 227
	spec.Pool = "okd"
	spec.Disks = append(spec.Disks, DiskSpec{Slot: "scsi1", Storage: "ceph-rbd", SizeGiB: 500, Serial: "worker-0-data"})
	return spec
}

func TestVMConfigDiff(t *testing.T) {
	spec := diffSpec()

	t.Run("rendered spec has no drift", func(t *testing.T) {
		if changes := renderedConfig(t, &spec).Diff(spec); len(changes) != 0 {
			t.Fatalf("Diff() = %+v, want none", changes)
		}
	})

	t.Run("mac case is ignored", func(t *testing.T) {
		c := renderedConfig(t, &spec)
		c.MAC = strings.ToLower(c.MAC)
		if changes := c.Diff(spec); len(changes) != 0 {
			t.Fatalf("Diff() = %+v, want none", changes)
		}
	})

	t.Run("tag order and case are ignored", func(t *testing.T) {
		c := renderedConfig(t, &spec)
		c.Tags = []string{"okd", "Grappleberry"}
		if changes := c.Diff(spec); len(changes) != 0 {
			t.Fatalf("Diff() = %+v, want none", changes)
		}
	})

	t.Run("memory needs a power cycle", func(t *testing.T) {
		c := renderedConfig(t, &spec)
		c.MemoryMiB = 8192
		want := []Change{{Field: "memory", Want: "16384", Have: "8192", NeedsPowerCycle: true}}
		if changes := c.Diff(spec); !reflect.DeepEqual(changes, want) {
			t.Fatalf("Diff() = %+v, want %+v", changes, want)
		}
	})

	t.Run("disk larger than spec is reported", func(t *testing.T) {
		c := renderedConfig(t, &spec)
		d := c.Disks["scsi0"]
		d.SizeGiB = 150
		c.Disks["scsi0"] = d
		want := []Change{{Field: "disks.scsi0.size", Want: "120G", Have: "150G"}}
		if changes := c.Diff(spec); !reflect.DeepEqual(changes, want) {
			t.Fatalf("Diff() = %+v, want %+v", changes, want)
		}
	})

	t.Run("missing disk is reported by slot", func(t *testing.T) {
		c := renderedConfig(t, &spec)
		delete(c.Disks, "scsi1")
		want := []Change{{Field: "disks.scsi1", Want: "ceph-rbd:500G", Have: ""}}
		if changes := c.Diff(spec); !reflect.DeepEqual(changes, want) {
			t.Fatalf("Diff() = %+v, want %+v", changes, want)
		}
	})

	t.Run("missing tag is added to the existing ones", func(t *testing.T) {
		c := renderedConfig(t, &spec)
		c.Tags = []string{"okd", "lab"}
		want := []Change{{Field: "tags", Want: "grappleberry;lab;okd", Have: "okd;lab"}}
		if changes := c.Diff(spec); !reflect.DeepEqual(changes, want) {
			t.Fatalf("Diff() = %+v, want %+v", changes, want)
		}
	})

	t.Run("detached iso drifts from a spec with one", func(t *testing.T) {
		c := renderedConfig(t, &spec)
		c.CDROM = ""
		c.BootOrder = []string{"scsi0"}
		want := []Change{
			{Field: "boot", Want: "order=scsi0;ide2", Have: "order=scsi0", NeedsPowerCycle: true},
			{Field: "ide2", Want: spec.ISOVolID, Have: ""},
		}
		if changes := c.Diff(spec); !reflect.DeepEqual(changes, want) {
			t.Fatalf("Diff() = %+v, want %+v", changes, want)
		}
	})

	t.Run("spec without iso matches a detached vm", func(t *testing.T) {
		noISO := spec
		noISO.ISOVolID = ""
		c := renderedConfig(t, &spec)
		c.CDROM = ""
		c.BootOrder = []string{"scsi0"}
		if changes := c.Diff(noISO); len(changes) != 0 {
			t.Fatalf("Diff() = %+v, want none", changes)
		}
	})

	t.Run("unset keys are not drift", func(t *testing.T) {
		c := VMConfig{Digest: "d", OnBoot: true, AgentEnabled: true, Tags: []string{"grappleberry"}, CDROM: spec.ISOVolID, Disks: renderedConfig(t, &spec).Disks}
		if changes := c.Diff(spec); len(changes) != 0 {
			t.Fatalf("Diff() = %+v, want none", changes)
		}
	})

	t.Run("field flags", func(t *testing.T) {
		cases := []struct {
			field  string
			mutate func(*VMConfig)
			want   Change
		}{
			{"name", func(c *VMConfig) { c.Name = "other" }, Change{Field: "name", Want: spec.Name, Have: "other"}},
			{"cores", func(c *VMConfig) { c.Cores = 2 }, Change{Field: "cores", Want: "4", Have: "2", NeedsPowerCycle: true}},
			{"sockets", func(c *VMConfig) { c.Sockets = 2 }, Change{Field: "sockets", Want: "1", Have: "2", NeedsPowerCycle: true}},
			{"cpu", func(c *VMConfig) { c.CPUType = "x86-64-v3" }, Change{Field: "cpu", Want: "host", Have: "x86-64-v3", NeedsPowerCycle: true}},
			{"machine", func(c *VMConfig) { c.Machine = "pc" }, Change{Field: "machine", Want: "q35", Have: "pc", NeedsPowerCycle: true}},
			{"bios", func(c *VMConfig) { c.BIOS = "seabios" }, Change{Field: "bios", Want: "ovmf", Have: "seabios", NeedsPowerCycle: true}},
			{"scsihw", func(c *VMConfig) { c.SCSIHW = "lsi" }, Change{Field: "scsihw", Want: "virtio-scsi-single", Have: "lsi", NeedsPowerCycle: true}},
			{"ostype", func(c *VMConfig) { c.OSType = "win11" }, Change{Field: "ostype", Want: "l26", Have: "win11", NeedsPowerCycle: true}},
			{"onboot", func(c *VMConfig) { c.OnBoot = false }, Change{Field: "onboot", Want: "1", Have: "0"}},
			{"balloon", func(c *VMConfig) { c.Balloon = 2048 }, Change{Field: "balloon", Want: "0", Have: "2048"}},
			{"agent", func(c *VMConfig) { c.AgentEnabled = false }, Change{Field: "agent", Want: "enabled=1", Have: "enabled=0", NeedsPowerCycle: true}},
			{"nic model", func(c *VMConfig) { c.NICModel = "e1000" }, Change{Field: "net0.model", Want: "virtio", Have: "e1000", NeedsPowerCycle: true}},
			{"mac", func(c *VMConfig) { c.MAC = "BC:24:11:00:00:01" }, Change{Field: "net0.mac", Want: spec.MAC, Have: "BC:24:11:00:00:01", NeedsPowerCycle: true}},
			{"bridge", func(c *VMConfig) { c.Bridge = "vmbr1" }, Change{Field: "net0.bridge", Want: "vmbr0", Have: "vmbr1"}},
			{"vlan", func(c *VMConfig) { c.VLAN = 0 }, Change{Field: "net0.vlan", Want: "227", Have: "0"}},
			{"efi storage", func(c *VMConfig) { c.EFI.Storage = "ceph-rbd" }, Change{Field: "efidisk0.storage", Want: "local-lvm", Have: "ceph-rbd"}},
			{"disk storage", func(c *VMConfig) { d := c.Disks["scsi0"]; d.Storage = "ceph-rbd"; c.Disks["scsi0"] = d }, Change{Field: "disks.scsi0.storage", Want: "local-lvm", Have: "ceph-rbd"}},
			{"disk smaller", func(c *VMConfig) { d := c.Disks["scsi0"]; d.SizeGiB = 100; c.Disks["scsi0"] = d }, Change{Field: "disks.scsi0.size", Want: "120G", Have: "100G"}},
			{"disk serial", func(c *VMConfig) { d := c.Disks["scsi0"]; d.Serial = "x"; c.Disks["scsi0"] = d }, Change{Field: "disks.scsi0.serial", Want: "worker-0-root", Have: "x", NeedsPowerCycle: true}},
			{"disk iothread", func(c *VMConfig) { d := c.Disks["scsi0"]; d.IOThread = false; c.Disks["scsi0"] = d }, Change{Field: "disks.scsi0.iothread", Want: "1", Have: "0", NeedsPowerCycle: true}},
			{"disk discard", func(c *VMConfig) { d := c.Disks["scsi0"]; d.Discard = false; c.Disks["scsi0"] = d }, Change{Field: "disks.scsi0.discard", Want: "on", Have: "ignore", NeedsPowerCycle: true}},
			{"cdrom", func(c *VMConfig) { c.CDROM = "local:iso/other.iso" }, Change{Field: "ide2", Want: spec.ISOVolID, Have: "local:iso/other.iso"}},
		}
		for _, tc := range cases {
			t.Run(tc.field, func(t *testing.T) {
				c := renderedConfig(t, &spec)
				tc.mutate(&c)
				if changes := c.Diff(spec); !reflect.DeepEqual(changes, []Change{tc.want}) {
					t.Fatalf("Diff() = %+v, want %+v", changes, tc.want)
				}
			})
		}
	})
}

func TestDriverVMConfig(t *testing.T) {
	f := newFakeAPI(t, nil)
	got, err := f.driver(t).VMConfig(t.Context(), ref101)
	if err != nil {
		t.Fatalf("VMConfig(): %v", err)
	}
	want := VMConfig{
		Name: "grappleberry-master-0", Cores: 4, Sockets: 1, CPUType: "host", MemoryMiB: 16384,
		BIOS: "ovmf", Machine: "q35", SCSIHW: "virtio-scsi-single", OSType: "l26", OnBoot: true, AgentEnabled: true,
		BootOrder: []string{"scsi0", "ide2"}, Tags: []string{"grappleberry"},
		NICModel: "virtio", MAC: "BC:24:11:AA:BB:CC", Bridge: "vmbr0",
		Disks: map[string]DiskConfig{"scsi0": {
			Storage: "local-lvm", VolID: "local-lvm:vm-101-disk-1", SizeGiB: 120, Serial: "master-0-root",
			IOThread: true, Discard: true, Cache: "none", AIO: "io_uring",
		}},
		EFI:    &EFIConfig{Storage: "local-lvm", VolID: "local-lvm:vm-101-disk-0", EFIType: "4m"},
		CDROM:  "local:iso/grappleberry-master.iso",
		Digest: "7f3a0c2e5b9d1e4f6a8c0b2d4f6e8a0c2e4f6a8c",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("VMConfig() =\n%+v\nwant\n%+v", got, want)
	}
	master := newSpec(101, "grappleberry")
	master.Name, master.ISOVolID = "grappleberry-master-0", "local:iso/grappleberry-master.iso"
	master.Disks[0].Serial = "master-0-root"
	if changes := got.Diff(master); len(changes) != 0 {
		t.Fatalf("Diff(master spec) = %+v, want none", changes)
	}

	t.Run("missing vm on real pve is not found", func(t *testing.T) {
		f := newFakeAPI(t, map[string]http.HandlerFunc{
			"GET /nodes/pve/qemu/999/config": pveErrorHandler("Configuration file 'nodes/pve/qemu-server/999.conf' does not exist"),
		})
		if _, err := f.driver(t).VMConfig(t.Context(), VMRef{Node: "pve", VMID: 999}); !errors.Is(err, ErrNotFound) {
			t.Fatalf("VMConfig() error = %v, want ErrNotFound", err)
		}
	})

	t.Run("null data is not found", func(t *testing.T) {
		f := newFakeAPI(t, map[string]http.HandlerFunc{
			"GET /nodes/pve/qemu/999/config": jsonHandler(http.StatusOK, `{"data":null}`),
		})
		if _, err := f.driver(t).VMConfig(t.Context(), VMRef{Node: "pve", VMID: 999}); !errors.Is(err, ErrNotFound) {
			t.Fatalf("VMConfig() error = %v, want ErrNotFound", err)
		}
	})

	t.Run("malformed config is an error", func(t *testing.T) {
		f := newFakeAPI(t, map[string]http.HandlerFunc{
			"GET /nodes/pve/qemu/101/config": jsonHandler(http.StatusOK, `{"data":{"digest":"d","memory":"lots"}}`),
		})
		if _, err := f.driver(t).VMConfig(t.Context(), ref101); err == nil || !strings.Contains(err.Error(), "memory") {
			t.Fatalf("VMConfig() error = %v, want a memory parse error", err)
		}
	})
}

func TestDriverVMPending(t *testing.T) {
	f := newFakeAPI(t, nil)
	got, err := f.driver(t).VMPending(t.Context(), ref101)
	if err != nil {
		t.Fatalf("VMPending(): %v", err)
	}
	want := []Pending{
		{Key: "memory", Value: "16384", Pending: "32768"},
		{Key: "ide2", Value: "local:iso/grappleberry-master.iso,media=cdrom,size=904416K", Delete: true},
		{Key: "tags", Value: "grappleberry", Pending: "grappleberry;okd"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("VMPending() =\n%+v\nwant\n%+v", got, want)
	}

	t.Run("nothing queued is empty", func(t *testing.T) {
		f := newFakeAPI(t, map[string]http.HandlerFunc{
			"GET /nodes/pve/qemu/101/pending": jsonHandler(http.StatusOK, `{"data":[{"key":"digest","value":"d"},{"key":"cores","value":4}]}`),
		})
		got, err := f.driver(t).VMPending(t.Context(), ref101)
		if err != nil || len(got) != 0 {
			t.Fatalf("VMPending() = %+v, %v; want none", got, err)
		}
	})

	t.Run("null data is not found", func(t *testing.T) {
		f := newFakeAPI(t, map[string]http.HandlerFunc{
			"GET /nodes/pve/qemu/999/pending": jsonHandler(http.StatusOK, `{"data":null}`),
		})
		if _, err := f.driver(t).VMPending(t.Context(), VMRef{Node: "pve", VMID: 999}); !errors.Is(err, ErrNotFound) {
			t.Fatalf("VMPending() error = %v, want ErrNotFound", err)
		}
	})
}
