package proxmox

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/executor"
)

const gibBytes = int64(1024 * 1024 * 1024)

type fakeDriver struct {
	Driver
	perms    Permissions
	version  Version
	nodes    []NodeInfo
	storages map[string][]StorageInfo
	bridges  map[string][]Bridge
	vms      []VMSummary
	configs  map[int]VMConfig
	errs     map[string]error
}

func (f *fakeDriver) Permissions(context.Context) (Permissions, error) {
	return f.perms, f.errs["permissions"]
}

func (f *fakeDriver) Version(context.Context) (Version, error) {
	return f.version, f.errs["version"]
}

func (f *fakeDriver) Nodes(context.Context) ([]NodeInfo, error) {
	return f.nodes, f.errs["nodes"]
}

func (f *fakeDriver) ListVMs(context.Context) ([]VMSummary, error) {
	return f.vms, f.errs["listvms"]
}

func (f *fakeDriver) Storages(_ context.Context, node string) ([]StorageInfo, error) {
	s, ok := f.storages[node]
	if !ok {
		return nil, errors.New("node " + node + " unknown")
	}
	return s, nil
}

func (f *fakeDriver) Bridges(_ context.Context, node string) ([]Bridge, error) {
	b, ok := f.bridges[node]
	if !ok {
		return nil, errors.New("node " + node + " unknown")
	}
	return b, nil
}

func (f *fakeDriver) VMConfig(_ context.Context, ref VMRef) (VMConfig, error) {
	if err := f.errs["config"]; err != nil && ref.VMID == 200 {
		return VMConfig{}, err
	}
	if i := slices.IndexFunc(f.vms, func(vm VMSummary) bool { return vm.VMID == ref.VMID }); i >= 0 && f.vms[i].Type != "qemu" {
		return VMConfig{}, ErrNotFound // the qemu config path knows no containers
	}
	return f.configs[ref.VMID], nil
}

type fakePinger struct {
	alive                 map[netip.Addr]bool
	errs                  map[netip.Addr]error
	inflight, maxInflight *atomic.Int32
}

func (p fakePinger) Alive(_ context.Context, addr netip.Addr) (bool, error) {
	if p.inflight != nil {
		n := p.inflight.Add(1)
		for old := p.maxInflight.Load(); n > old && !p.maxInflight.CompareAndSwap(old, n); old = p.maxInflight.Load() {
		}
		time.Sleep(30 * time.Millisecond)
		p.inflight.Add(-1)
	}
	return p.alive[addr], p.errs[addr]
}

func goodStorages() []StorageInfo {
	return []StorageInfo{
		{Name: "ceph", Type: "rbd", Shared: true, Active: true, Content: []string{"images", "rootdir"}, AvailBytes: 500 * gibBytes, TotalBytes: 1000 * gibBytes},
		{Name: "local", Type: "dir", Active: true, Content: []string{"iso", "vztmpl", "backup"}, AvailBytes: 20 * gibBytes, TotalBytes: 100 * gibBytes},
	}
}

func goodDriver() *fakeDriver {
	flags := []string{"fpu", "avx2", "bmi2", "fma", "movbe", "sse4_2"}
	return &fakeDriver{
		perms: Permissions{
			"/": {"Sys.Audit": true, "Sys.Console": true},
			"/vms": {
				"VM.Allocate": true, "VM.Audit": true, "VM.PowerMgmt": true,
				"VM.Config.CDROM": true, "VM.Config.CPU": true, "VM.Config.Disk": true, "VM.Config.HWType": true,
				"VM.Config.Memory": true, "VM.Config.Network": true, "VM.Config.Options": true,
			},
			"/storage":                      {"Datastore.Allocate": true, "Datastore.AllocateSpace": true, "Datastore.AllocateTemplate": true, "Datastore.Audit": true},
			"/sdn/zones/localnetwork/vmbr0": {"SDN.Use": true},
			"/pool/okd":                     {"Pool.Allocate": true},
		},
		version: Version{Version: "9.0.10", Release: "9.0"},
		nodes: []NodeInfo{
			{Name: "pve1", Status: "online", MemTotalMiB: 65536, CPUFlags: flags},
			{Name: "pve2", Status: "online", MemTotalMiB: 65536, CPUFlags: flags},
		},
		storages: map[string][]StorageInfo{"pve1": goodStorages(), "pve2": goodStorages()},
		bridges: map[string][]Bridge{
			"pve1": {{Name: "vmbr0", VLANAware: true, Active: true}},
			"pve2": {{Name: "vmbr0", VLANAware: true, Active: true}},
		},
		vms: []VMSummary{
			{VMRef: VMRef{Node: "pve1", VMID: 100}, Type: "qemu", Name: "web", Status: "running", MaxMemMiB: 8192},
			{VMRef: VMRef{Node: "pve2", VMID: 200}, Type: "qemu", Name: "db", Status: "stopped", MaxMemMiB: 16384},
		},
		configs: map[int]VMConfig{
			100: {MAC: "BC:24:11:AA:BB:01", MACs: []string{"BC:24:11:AA:BB:01"}},
			200: {MAC: "bc:24:11:aa:bb:02", MACs: []string{"bc:24:11:aa:bb:02"}},
		},
		errs: map[string]error{},
	}
}

func goodInput() PreflightInput {
	return PreflightInput{
		Nodes: []string{"pve1", "pve2"}, Storage: "ceph", ISOStorage: "local", Bridge: "vmbr0", VLAN: 100,
		Pool: "okd", HA: true, Tag: "okd-grappleberry",
		VMIDs: []int{101, 102, 103},
		MACs:  []string{"BC:24:11:00:00:01", "BC:24:11:00:00:02", "BC:24:11:00:00:03"},
		NodeAddresses: map[netip.Addr]int{
			netip.MustParseAddr("10.0.0.10"): 101, netip.MustParseAddr("10.0.0.11"): 102, netip.MustParseAddr("10.0.0.12"): 103,
		},
		VIPs:            []netip.Addr{netip.MustParseAddr("10.0.0.5"), netip.MustParseAddr("10.0.0.6")},
		MemoryMiBByNode: map[string]int{"pve1": 32768, "pve2": 32768},
		ISOBytes:        2 * gibBytes,
		CPUModel:        "host",
	}
}

func TestPreflight(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(d *fakeDriver, in *PreflightInput, p *fakePinger)
		want   []Finding
	}{
		{name: "all green", mutate: func(*fakeDriver, *PreflightInput, *fakePinger) {}},
		{
			name:   "permissions missing privilege",
			mutate: func(d *fakeDriver, _ *PreflightInput, _ *fakePinger) { delete(d.perms["/vms"], "VM.PowerMgmt") },
			want:   []Finding{{"permissions", "fail", "missing privilege VM.PowerMgmt on /vms or /pool/okd"}},
		},
		{
			name: "permissions missing privilege without a pool",
			mutate: func(d *fakeDriver, in *PreflightInput, _ *fakePinger) {
				delete(d.perms["/vms"], "VM.PowerMgmt")
				in.Pool = ""
			},
			want: []Finding{{"permissions", "fail", "missing privilege VM.PowerMgmt on /vms"}},
		},
		{
			name: "permissions vm privilege granted on the pool",
			mutate: func(d *fakeDriver, _ *PreflightInput, _ *fakePinger) {
				delete(d.perms["/vms"], "VM.PowerMgmt")
				d.perms["/pool/okd"]["VM.PowerMgmt"] = false
			},
		},
		{
			name: "permissions ancestor lacks datastore allocate",
			mutate: func(d *fakeDriver, _ *PreflightInput, _ *fakePinger) {
				delete(d.perms["/storage"], "Datastore.Allocate")
			},
			want: []Finding{{"permissions", "fail", "missing privilege Datastore.Allocate on /storage/local"}},
		},
		{
			name: "permissions ancestor grant without propagate stops there",
			mutate: func(d *fakeDriver, _ *PreflightInput, _ *fakePinger) {
				d.perms["/storage"]["Datastore.Allocate"] = false
			},
			want: []Finding{{"permissions", "fail", "missing privilege Datastore.Allocate on /storage/local"}},
		},
		{
			name: "permissions exact path grant counts whatever its propagate flag",
			mutate: func(d *fakeDriver, _ *PreflightInput, _ *fakePinger) {
				delete(d.perms["/"], "Sys.Audit")
				d.perms["/nodes/pve1"] = map[string]bool{"Sys.Audit": false}
				d.perms["/nodes/pve2"] = map[string]bool{"Sys.Audit": false}
			},
		},
		{
			name: "permissions conditional privileges skipped",
			mutate: func(d *fakeDriver, in *PreflightInput, _ *fakePinger) {
				delete(d.perms["/"], "Sys.Console")
				delete(d.perms, "/pool/okd")
				in.HA, in.Pool = false, ""
			},
		},
		{
			name: "permissions pool needs allocate",
			mutate: func(d *fakeDriver, _ *PreflightInput, _ *fakePinger) {
				d.perms["/pool/okd"] = map[string]bool{"Pool.Audit": true}
			},
			want: []Finding{{"permissions", "fail", "missing privilege Pool.Allocate on /pool/okd"}},
		},
		{
			name: "permissions vlan checks the tagged sdn path",
			mutate: func(d *fakeDriver, _ *PreflightInput, _ *fakePinger) {
				d.perms["/sdn/zones/localnetwork/vmbr0"]["SDN.Use"] = false
			},
			want: []Finding{{"permissions", "fail", "missing privilege SDN.Use on /sdn/zones/localnetwork/vmbr0/100"}},
		},
		{
			name: "permissions sdn grant on the tag alone suffices",
			mutate: func(d *fakeDriver, _ *PreflightInput, _ *fakePinger) {
				delete(d.perms, "/sdn/zones/localnetwork/vmbr0")
				d.perms["/sdn/zones/localnetwork/vmbr0/100"] = map[string]bool{"SDN.Use": false}
			},
		},
		{
			name: "permissions single storage reports each privilege once",
			mutate: func(d *fakeDriver, in *PreflightInput, _ *fakePinger) {
				in.ISOStorage = "ceph"
				delete(d.perms["/storage"], "Datastore.Audit")
				for _, n := range in.Nodes {
					d.storages[n][0].Content = append(d.storages[n][0].Content, "iso")
				}
			},
			want: []Finding{{"permissions", "fail", "missing privilege Datastore.Audit on /storage/ceph"}},
		},
		{
			name:   "version too old",
			mutate: func(d *fakeDriver, _ *PreflightInput, _ *fakePinger) { d.version.Version = "7.4-1" },
			want:   []Finding{{"version", "fail", "proxmox ve 7.4-1 unsupported"}},
		},
		{
			name:   "version 8 accepted",
			mutate: func(d *fakeDriver, _ *PreflightInput, _ *fakePinger) { d.version.Version = "8.2.4" },
		},
		{
			name:   "version unparseable",
			mutate: func(d *fakeDriver, _ *PreflightInput, _ *fakePinger) { d.version.Version = "" },
			want:   []Finding{{"version", "fail", "unsupported"}},
		},
		{
			name:   "cpu flags missing",
			mutate: func(d *fakeDriver, _ *PreflightInput, _ *fakePinger) { d.nodes[1].CPUFlags = []string{"avx2", "fma"} },
			want:   []Finding{{"cpu-flags", "fail", "node pve2 lacks cpu flags bmi2, movbe"}},
		},
		{
			name:   "cpu flags unknown when the node status was not readable",
			mutate: func(d *fakeDriver, _ *PreflightInput, _ *fakePinger) { d.nodes[1].CPUFlags = nil },
			want:   []Finding{{"cpu-flags", "fail", "node pve2: status not readable"}},
		},
		{
			name: "placement node offline",
			mutate: func(d *fakeDriver, _ *PreflightInput, _ *fakePinger) {
				d.nodes[1].Status, d.nodes[1].CPUFlags = "offline", nil
			},
			want: []Finding{{"cpu-flags", "fail", "node pve2 is offline"}},
		},
		{
			name:   "placement node not in cluster",
			mutate: func(_ *fakeDriver, in *PreflightInput, _ *fakePinger) { in.Nodes = append(in.Nodes, "pve3") },
			want: []Finding{
				{"cpu-flags", "fail", "node pve3 not in cluster"},
				{"storage-content", "fail", "node pve3: list storages: node pve3 unknown"},
				{"bridge", "fail", "node pve3: list bridges: node pve3 unknown"},
			},
		},
		{
			name:   "memory overcommitted",
			mutate: func(_ *fakeDriver, in *PreflightInput, _ *fakePinger) { in.MemoryMiBByNode["pve1"] = 49153 },
			want:   []Finding{{"memory", "warn", "node pve1: planned 49153 MiB + running 8192 MiB + 8192 MiB reserve exceeds 65536 MiB"}},
		},
		{
			name: "memory skips the cluster's own running vms",
			mutate: func(d *fakeDriver, in *PreflightInput, _ *fakePinger) {
				d.vms[0].Tags = []string{"okd", "okd-grappleberry"}
				in.MemoryMiBByNode["pve1"] = 49153
			},
		},
		{
			name: "memory counts the cluster's own vm the plan moves away",
			mutate: func(d *fakeDriver, in *PreflightInput, _ *fakePinger) {
				d.vms[0].Tags = []string{"okd-grappleberry"}
				d.vms[0].MaxMemMiB = 57345
				in.MemoryMiBByNode["pve1"] = 4096
			},
			want: []Finding{{"memory", "warn", "node pve1: planned 4096 MiB + running 53249 MiB + 8192 MiB reserve exceeds 65536 MiB"}},
		},
		{
			name:   "memory exactly fits",
			mutate: func(_ *fakeDriver, in *PreflightInput, _ *fakePinger) { in.MemoryMiBByNode["pve1"] = 49152 },
		},
		{
			name:   "iso space short",
			mutate: func(_ *fakeDriver, in *PreflightInput, _ *fakePinger) { in.ISOBytes = 11 * gibBytes },
			want: []Finding{
				{"iso-space", "fail", "storage local on node pve1 has 20480 MiB free; need 22528 MiB"},
				{"iso-space", "fail", "storage local on node pve2 has 20480 MiB free; need 22528 MiB"},
			},
		},
		{
			name: "iso space on shared storage is reported once",
			mutate: func(d *fakeDriver, in *PreflightInput, _ *fakePinger) {
				in.ISOBytes = 11 * gibBytes
				for _, n := range in.Nodes {
					d.storages[n][1].Shared = true
				}
			},
			want: []Finding{{"iso-space", "fail", "storage local (shared) has 20480 MiB free; need 22528 MiB"}},
		},
		{
			name:   "vmid collision",
			mutate: func(_ *fakeDriver, in *PreflightInput, _ *fakePinger) { in.VMIDs = append(in.VMIDs, 200, 100) },
			want: []Finding{
				{"vmid-collision", "fail", `vmid 100 already used by "web" on node pve1`},
				{"vmid-collision", "fail", `vmid 200 already used by "db" on node pve2`},
			},
		},
		{
			name: "vmid collision with a container",
			mutate: func(d *fakeDriver, in *PreflightInput, _ *fakePinger) {
				d.vms = append(d.vms, VMSummary{VMRef: VMRef{Node: "pve1", VMID: 300}, Type: "lxc", Name: "dns", Status: "running", MaxMemMiB: 512})
				in.VMIDs = append(in.VMIDs, 300)
			},
			want: []Finding{{"vmid-collision", "fail", `vmid 300 already used by "dns" on node pve1`}},
		},
		{
			name: "vmid collision skips the cluster's own vms",
			mutate: func(d *fakeDriver, in *PreflightInput, _ *fakePinger) {
				d.vms[1].Tags = []string{"okd-grappleberry"}
				in.VMIDs = append(in.VMIDs, 200)
			},
		},
		{
			name: "vmid collision with a prefix tag is foreign",
			mutate: func(d *fakeDriver, in *PreflightInput, _ *fakePinger) {
				d.vms[1].Tags = []string{"okd-grappleberry-old"}
				in.VMIDs = append(in.VMIDs, 200)
			},
			want: []Finding{{"vmid-collision", "fail", `vmid 200 already used by "db" on node pve2`}},
		},
		{
			name: "mac collision skips the cluster's own vms",
			mutate: func(d *fakeDriver, in *PreflightInput, _ *fakePinger) {
				d.vms[0].Tags = []string{"okd-grappleberry"}
				in.MACs[0] = "bc:24:11:aa:bb:01"
			},
		},
		{
			name:   "mac collision is case-insensitive",
			mutate: func(_ *fakeDriver, in *PreflightInput, _ *fakePinger) { in.MACs[0] = "bc:24:11:aa:bb:01" },
			want:   []Finding{{"mac-collision", "fail", "mac bc:24:11:aa:bb:01 already used by vmid 100 on node pve1"}},
		},
		{
			name: "mac collision on secondary nic",
			mutate: func(d *fakeDriver, _ *PreflightInput, _ *fakePinger) {
				d.configs[200] = VMConfig{MAC: "bc:24:11:aa:bb:02", MACs: []string{"bc:24:11:aa:bb:02", "bc:24:11:00:00:03"}}
			},
			want: []Finding{{"mac-collision", "fail", "mac BC:24:11:00:00:03 already used by vmid 200 on node pve2"}},
		},
		{
			name: "mac collision reads no configs without planned macs",
			mutate: func(d *fakeDriver, in *PreflightInput, _ *fakePinger) {
				in.MACs = nil
				d.errs["config"] = errors.New("boom")
			},
		},
		{
			name:   "mac config read error warns",
			mutate: func(d *fakeDriver, _ *PreflightInput, _ *fakePinger) { d.errs["config"] = errors.New("boom") },
			want:   []Finding{{"mac-collision", "warn", "read config of vmid 200 on node pve2: boom"}},
		},
		{
			name: "address in use",
			mutate: func(_ *fakeDriver, _ *PreflightInput, p *fakePinger) {
				p.alive[netip.MustParseAddr("10.0.0.5")] = true
			},
			want: []Finding{{"address-in-use", "fail", "address 10.0.0.5 answers ping"}},
		},
		{
			name: "addresses in use are reported in address order",
			mutate: func(_ *fakeDriver, _ *PreflightInput, p *fakePinger) {
				for _, a := range []string{"10.0.0.11", "10.0.0.5", "10.0.0.10"} {
					p.alive[netip.MustParseAddr(a)] = true
				}
			},
			want: []Finding{
				{"address-in-use", "fail", "address 10.0.0.5 answers ping"},
				{"address-in-use", "fail", "address 10.0.0.10 answers ping"},
				{"address-in-use", "fail", "address 10.0.0.11 answers ping"},
			},
		},
		{
			name: "addresses of the cluster's own running vms are skipped",
			mutate: func(d *fakeDriver, _ *PreflightInput, p *fakePinger) {
				d.vms[0].VMID, d.vms[0].Tags = 101, []string{"okd-grappleberry"}
				p.alive[netip.MustParseAddr("10.0.0.10")] = true
				p.alive[netip.MustParseAddr("10.0.0.5")] = true
				p.alive[netip.MustParseAddr("10.0.0.11")] = true
			},
			want: []Finding{{"address-in-use", "fail", "address 10.0.0.11 answers ping"}},
		},
		{
			name: "addresses of the cluster's own stopped vms still count",
			mutate: func(d *fakeDriver, _ *PreflightInput, p *fakePinger) {
				d.vms[0].VMID, d.vms[0].Tags, d.vms[0].Status = 101, []string{"okd-grappleberry"}, "stopped"
				p.alive[netip.MustParseAddr("10.0.0.10")] = true
				p.alive[netip.MustParseAddr("10.0.0.5")] = true
			},
			want: []Finding{
				{"address-in-use", "fail", "address 10.0.0.5 answers ping"},
				{"address-in-use", "fail", "address 10.0.0.10 answers ping"},
			},
		},
		{
			name: "address check inconclusive warns per cause",
			mutate: func(_ *fakeDriver, _ *PreflightInput, p *fakePinger) {
				p.alive[netip.MustParseAddr("10.0.0.6")] = true
				p.errs[netip.MustParseAddr("10.0.0.11")] = errors.New("ping: executable file not found")
				p.errs[netip.MustParseAddr("10.0.0.5")] = errors.New("ping: executable file not found")
				p.errs[netip.MustParseAddr("10.0.0.10")] = errors.New("ping exit 2: connect: Network is unreachable")
			},
			want: []Finding{
				{"address-in-use", "fail", "address 10.0.0.6 answers ping"},
				{"address-in-use", "warn", "address check inconclusive for 10.0.0.5, 10.0.0.11: ping: executable file not found"},
				{"address-in-use", "warn", "address check inconclusive for 10.0.0.10: ping exit 2: connect: Network is unreachable"},
			},
		},
		{
			name:   "storage missing",
			mutate: func(_ *fakeDriver, in *PreflightInput, _ *fakePinger) { in.Storage = "nfs" },
			want: []Finding{
				{"storage-content", "fail", "storage nfs not found on node pve1"},
				{"storage-content", "fail", "storage nfs not found on node pve2"},
			},
		},
		{
			name: "storage inactive",
			mutate: func(d *fakeDriver, _ *PreflightInput, _ *fakePinger) {
				d.storages["pve2"][0].Active = false
			},
			want: []Finding{{"storage-content", "fail", "storage ceph inactive on node pve2"}},
		},
		{
			name: "iso storage lacks iso content",
			mutate: func(d *fakeDriver, _ *PreflightInput, _ *fakePinger) {
				d.storages["pve1"][1].Content = []string{"vztmpl"}
			},
			want: []Finding{{"storage-content", "fail", "storage local on node pve1 lacks iso content"}},
		},
		{
			name:   "bridge missing",
			mutate: func(_ *fakeDriver, in *PreflightInput, _ *fakePinger) { in.Bridge = "vmbr1" },
			want: []Finding{
				{"permissions", "fail", "missing privilege SDN.Use on /sdn/zones/localnetwork/vmbr1/100"},
				{"bridge", "fail", "bridge vmbr1 not found on node pve1"},
				{"bridge", "fail", "bridge vmbr1 not found on node pve2"},
			},
		},
		{
			name:   "bridge inactive",
			mutate: func(d *fakeDriver, _ *PreflightInput, _ *fakePinger) { d.bridges["pve1"][0].Active = false },
			want:   []Finding{{"bridge", "fail", "bridge vmbr0 inactive on node pve1"}},
		},
		{
			name:   "bridge not vlan-aware",
			mutate: func(d *fakeDriver, _ *PreflightInput, _ *fakePinger) { d.bridges["pve2"][0].VLANAware = false },
			want:   []Finding{{"bridge", "fail", "bridge vmbr0 on node pve2 is not vlan-aware"}},
		},
		{
			name: "bridge vlan-awareness irrelevant without vlan",
			mutate: func(d *fakeDriver, in *PreflightInput, _ *fakePinger) {
				d.bridges["pve2"][0].VLANAware = false
				in.VLAN = 0
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, in, p := goodDriver(), goodInput(), fakePinger{alive: map[netip.Addr]bool{}, errs: map[netip.Addr]error{}}
			tc.mutate(d, &in, &p)
			got, err := Preflight(context.Background(), d, p, in)
			if err != nil {
				t.Fatalf("Preflight: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %d findings %+v, want %d %+v", len(got), got, len(tc.want), tc.want)
			}
			for i, w := range tc.want {
				g := got[i]
				if g.Check != w.Check || g.Severity != w.Severity || !strings.Contains(g.Message, w.Message) {
					t.Errorf("finding %d: got %+v, want %+v", i, g, w)
				}
			}
		})
	}
}

func TestPreflightPingsConcurrently(t *testing.T) {
	p := fakePinger{alive: map[netip.Addr]bool{}, inflight: new(atomic.Int32), maxInflight: new(atomic.Int32)}
	if _, err := Preflight(context.Background(), goodDriver(), p, goodInput()); err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	if got := p.maxInflight.Load(); got < 2 {
		t.Fatalf("at most %d pings in flight, want them to overlap", got)
	}
}

func TestPreflightNilPingerSkipsAddresses(t *testing.T) {
	got, err := Preflight(context.Background(), goodDriver(), nil, goodInput())
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("findings = %+v, want none", got)
	}
}

func TestFixtureNodeHasRequiredCPUFlags(t *testing.T) {
	nodes, err := newFakeAPI(t, nil).driver(t).Nodes(t.Context())
	if err != nil {
		t.Fatalf("Nodes(): %v", err)
	}
	for _, flag := range requiredCPUFlags {
		if !slices.Contains(nodes[0].CPUFlags, flag) {
			t.Errorf("fixture node lacks cpu flag %s", flag)
		}
	}
}

func TestPreflightDependencyErrors(t *testing.T) {
	for _, key := range []string{"permissions", "version", "nodes", "listvms"} {
		t.Run(key, func(t *testing.T) {
			d := goodDriver()
			d.errs[key] = errors.New("down")
			if _, err := Preflight(context.Background(), d, fakePinger{}, goodInput()); err == nil || !strings.Contains(err.Error(), "down") {
				t.Fatalf("want wrapped error, got %v", err)
			}
		})
	}
}

func TestExecPinger(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	_, noReply := pingFlavour(runtime.GOOS)
	exitNoReply := fmt.Sprintf("exit %d\n", noReply)
	cases := []struct {
		name    string
		bin     string
		alive   bool
		wantErr string
	}{
		{name: "up", bin: write("up", "exit 0\n"), alive: true},
		{name: "no reply", bin: write("down", exitNoReply)},
		{name: "missing", bin: filepath.Join(dir, "nope"), wantErr: "nope"},
		{name: "other exit", bin: write("other", "echo usage >&2\nexit 64\n"), wantErr: "ping exit 64: usage"},
		{name: "unreachable", bin: write("unreach", "echo 'ping: sendto: No route to host' >&2\n"+exitNoReply), wantErr: "No route to host"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := execPinger{ex: executor.New(), bin: tc.bin}
			alive, err := p.Alive(context.Background(), netip.MustParseAddr("10.0.0.5"))
			if alive != tc.alive || (err != nil) != (tc.wantErr != "") || (err != nil && !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("Alive = %v, %v; want %v, error containing %q", alive, err, tc.alive, tc.wantErr)
			}
		})
	}
}
