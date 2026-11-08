package proxmox

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	return f.configs[ref.VMID], nil
}

type fakePinger struct{ alive map[netip.Addr]bool }

func (p fakePinger) Alive(_ context.Context, addr netip.Addr) bool { return p.alive[addr] }

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
			"/": {"Sys.Audit": true, "Sys.Console": true, "Sys.Modify": true},
			"/vms": {
				"VM.Allocate": true, "VM.Audit": true, "VM.PowerMgmt": true,
				"VM.Config.CDROM": true, "VM.Config.CPU": true, "VM.Config.Disk": true, "VM.Config.HWType": true,
				"VM.Config.Memory": true, "VM.Config.Network": true, "VM.Config.Options": true,
			},
			"/storage":                      {"Datastore.Allocate": true, "Datastore.AllocateSpace": true, "Datastore.AllocateTemplate": true, "Datastore.Audit": true},
			"/sdn/zones/localnetwork/vmbr0": {"SDN.Use": true},
			"/pool/okd":                     {"Pool.Audit": true},
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
			{VMRef: VMRef{Node: "pve1", VMID: 100}, Name: "web", Status: "running", MaxMemMiB: 8192},
			{VMRef: VMRef{Node: "pve2", VMID: 200}, Name: "db", Status: "stopped", MaxMemMiB: 16384},
		},
		configs: map[int]VMConfig{
			100: {MAC: "BC:24:11:AA:BB:01"},
			200: {MAC: "bc:24:11:aa:bb:02"},
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
		Addresses: []netip.Addr{
			netip.MustParseAddr("10.0.0.10"), netip.MustParseAddr("10.0.0.11"),
			netip.MustParseAddr("10.0.0.5"), netip.MustParseAddr("10.0.0.6"),
		},
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
			want:   []Finding{{"permissions", "fail", "missing privilege VM.PowerMgmt on /vms"}},
		},
		{
			name: "permissions ancestor lacks datastore allocate",
			mutate: func(d *fakeDriver, _ *PreflightInput, _ *fakePinger) {
				delete(d.perms["/storage"], "Datastore.Allocate")
			},
			want: []Finding{{"permissions", "fail", "missing privilege Datastore.Allocate on /storage/local"}},
		},
		{
			name: "permissions exact path beats ancestor",
			mutate: func(d *fakeDriver, _ *PreflightInput, _ *fakePinger) {
				delete(d.perms["/"], "Sys.Audit")
				d.perms["/nodes/pve1"] = map[string]bool{"Sys.Audit": true}
			},
			want: []Finding{{"permissions", "fail", "missing privilege Sys.Audit on /nodes/pve2"}},
		},
		{
			name: "permissions conditional privileges skipped",
			mutate: func(d *fakeDriver, in *PreflightInput, _ *fakePinger) {
				delete(d.perms["/"], "Sys.Console")
				delete(d.perms["/"], "Sys.Modify")
				delete(d.perms, "/pool/okd")
				in.HA, in.Tag, in.Pool = false, "", ""
			},
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
			name:   "vmid collision",
			mutate: func(_ *fakeDriver, in *PreflightInput, _ *fakePinger) { in.VMIDs = append(in.VMIDs, 200, 100) },
			want: []Finding{
				{"vmid-collision", "fail", `vmid 100 already used by "web" on node pve1`},
				{"vmid-collision", "fail", `vmid 200 already used by "db" on node pve2`},
			},
		},
		{
			name:   "mac collision is case-insensitive",
			mutate: func(_ *fakeDriver, in *PreflightInput, _ *fakePinger) { in.MACs[0] = "bc:24:11:aa:bb:01" },
			want:   []Finding{{"mac-collision", "fail", "mac bc:24:11:aa:bb:01 already used by vmid 100 on node pve1"}},
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
				{"permissions", "fail", "missing privilege SDN.Use on /sdn/zones/localnetwork/vmbr1"},
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
			d, in, p := goodDriver(), goodInput(), fakePinger{alive: map[netip.Addr]bool{}}
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
	addr := netip.MustParseAddr("10.0.0.5")
	ctx := context.Background()
	ex := executor.New()
	cases := []struct {
		name string
		bin  string
		want bool
	}{
		{"up", write("up", "exit 0\n"), true},
		{"down", write("down", "exit 1\n"), false},
		{"missing", filepath.Join(dir, "nope"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := execPinger{ex: ex, bin: tc.bin}
			if got := p.Alive(ctx, addr); got != tc.want {
				t.Fatalf("Alive = %v, want %v", got, tc.want)
			}
		})
	}
	if ExecPinger(ex).(execPinger).bin != "ping" {
		t.Fatal("ExecPinger must run ping")
	}
}

func TestPingWait(t *testing.T) {
	if got := pingWait("darwin"); got != "1000" {
		t.Fatalf("darwin: %q", got)
	}
	if got := pingWait("linux"); got != "1" {
		t.Fatalf("linux: %q", got)
	}
}
