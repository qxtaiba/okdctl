package proxmox

import (
	"context"
	"fmt"
	"maps"
	"net/netip"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/qxtaiba/okdctl/internal/executor"
)

// PreflightInput is the planned cluster footprint Preflight checks the
// Proxmox cluster against; Addresses carries every node address and both VIPs.
type PreflightInput struct {
	Nodes           []string
	Storage         string
	ISOStorage      string
	Bridge          string
	VLAN            int
	Pool            string
	HA              bool
	Tag             string
	VMIDs           []int
	MACs            []string
	Addresses       []netip.Addr
	MemoryMiBByNode map[string]int
	ISOBytes        int64
	CPUModel        string
}

// Finding is one preflight result; Severity is "fail" or "warn".
type Finding struct {
	Check    string
	Severity string
	Message  string
}

// Pinger reports whether an address answers on the network.
type Pinger interface {
	Alive(ctx context.Context, addr netip.Addr) bool
}

const (
	severityFail = "fail"
	severityWarn = "warn"

	memoryReserveMiB = 8192
)

var (
	requiredCPUFlags = []string{"avx2", "bmi2", "fma", "movbe"}
	supportedMajors  = []int{8, 9}
	vmPrivileges     = []string{
		"VM.Allocate", "VM.Config.CDROM", "VM.Config.CPU", "VM.Config.Disk", "VM.Config.HWType",
		"VM.Config.Memory", "VM.Config.Network", "VM.Config.Options", "VM.PowerMgmt", "VM.Audit",
	}
	checkSeverity = map[string]string{
		"permissions": severityFail, "version": severityFail, "cpu-flags": severityFail,
		"memory": severityWarn, "iso-space": severityFail, "vmid-collision": severityFail,
		"mac-collision": severityFail, "address-in-use": severityFail,
		"storage-content": severityFail, "bridge": severityFail,
	}
)

type findings []Finding

func (f *findings) add(check, format string, args ...any) {
	f.emit(check, checkSeverity[check], format, args...)
}

func (f *findings) warn(check, format string, args ...any) {
	f.emit(check, severityWarn, format, args...)
}

func (f *findings) emit(check, sev, format string, args ...any) {
	*f = append(*f, Finding{Check: check, Severity: sev, Message: fmt.Sprintf(format, args...)})
}

type nodeInventory struct {
	storages    []StorageInfo
	storagesErr error
	bridges     []Bridge
	bridgesErr  error
}

type preflightState struct {
	in      PreflightInput
	perms   Permissions
	version Version
	nodes   map[string]NodeInfo
	vms     []VMSummary
	perNode map[string]nodeInventory
}

// Preflight runs every check against the cluster and returns the findings
// in check order; it errors only when an inventory call every check needs fails.
//
//nolint:gocritic // hugeParam: the brief fixes the by-value signature
func Preflight(ctx context.Context, d Driver, ping Pinger, in PreflightInput) ([]Finding, error) {
	st := &preflightState{in: in, nodes: map[string]NodeInfo{}, perNode: map[string]nodeInventory{}}
	if err := st.gather(ctx, d); err != nil {
		return nil, err
	}
	var out findings
	st.checkPermissions(&out)
	st.checkVersion(&out)
	st.checkCPUFlags(&out)
	st.checkMemory(&out)
	st.checkISOSpace(&out)
	st.checkVMIDCollision(&out)
	st.checkMACCollision(ctx, d, &out)
	st.checkAddresses(ctx, ping, &out)
	st.checkStorageContent(&out)
	st.checkBridge(&out)
	return out, nil
}

func (st *preflightState) gather(ctx context.Context, d Driver) error {
	var err error
	if st.perms, err = d.Permissions(ctx); err != nil {
		return fmt.Errorf("read permissions: %w", err)
	}
	if st.version, err = d.Version(ctx); err != nil {
		return fmt.Errorf("read version: %w", err)
	}
	nodes, err := d.Nodes(ctx)
	if err != nil {
		return fmt.Errorf("list nodes: %w", err)
	}
	for _, n := range nodes {
		st.nodes[n.Name] = n
	}
	if st.vms, err = d.ListVMs(ctx); err != nil {
		return fmt.Errorf("list vms: %w", err)
	}
	slices.SortFunc(st.vms, func(a, b VMSummary) int { return a.VMID - b.VMID })
	for _, node := range st.in.Nodes {
		var inv nodeInventory
		inv.storages, inv.storagesErr = d.Storages(ctx, node)
		inv.bridges, inv.bridgesErr = d.Bridges(ctx, node)
		st.perNode[node] = inv
	}
	return nil
}

type privReq struct{ path, priv string }

func (st *preflightState) requiredPrivileges() []privReq {
	in := st.in
	var reqs []privReq
	for _, p := range vmPrivileges {
		reqs = append(reqs, privReq{"/vms", p})
	}
	reqs = append(reqs,
		privReq{"/storage/" + in.Storage, "Datastore.AllocateSpace"},
		privReq{"/storage/" + in.Storage, "Datastore.Audit"},
		privReq{"/storage/" + in.ISOStorage, "Datastore.AllocateTemplate"},
		privReq{"/storage/" + in.ISOStorage, "Datastore.Allocate"},
		privReq{"/storage/" + in.ISOStorage, "Datastore.Audit"},
		privReq{"/sdn/zones/localnetwork/" + in.Bridge, "SDN.Use"},
	)
	for _, n := range in.Nodes {
		reqs = append(reqs, privReq{"/nodes/" + n, "Sys.Audit"})
	}
	if in.HA {
		reqs = append(reqs, privReq{"/", "Sys.Console"})
	}
	if in.Pool != "" {
		reqs = append(reqs, privReq{"/pool/" + in.Pool, "Pool.Audit"})
	}
	if in.Tag != "" {
		reqs = append(reqs, privReq{"/", "Sys.Modify"})
	}
	return reqs
}

func (st *preflightState) checkPermissions(out *findings) {
	for _, r := range st.requiredPrivileges() {
		if !st.perms.granted(r.path, r.priv) {
			out.add("permissions", "missing privilege %s on %s", r.priv, r.path)
		}
	}
}

// granted walks from path to "/" because PVE ACLs propagate downwards and
// /access/permissions lists only the paths that carry an entry.
func (p Permissions) granted(path, priv string) bool {
	for {
		if p[path][priv] {
			return true
		}
		if path == "/" || path == "" {
			return false
		}
		if i := strings.LastIndex(path, "/"); i > 0 {
			path = path[:i]
		} else {
			path = "/"
		}
	}
}

func (st *preflightState) checkVersion(out *findings) {
	majorStr, _, _ := strings.Cut(st.version.Version, ".")
	major, err := strconv.Atoi(majorStr)
	if err != nil || !slices.Contains(supportedMajors, major) {
		out.add("version", "proxmox ve %s unsupported; need major 8 or 9", st.version.Version)
	}
}

func (st *preflightState) checkCPUFlags(out *findings) {
	for _, name := range st.in.Nodes {
		node, ok := st.nodes[name]
		if !ok {
			out.add("cpu-flags", "node %s not in cluster", name)
			continue
		}
		var missing []string
		for _, f := range requiredCPUFlags {
			if !slices.Contains(node.CPUFlags, f) {
				missing = append(missing, f)
			}
		}
		if len(missing) > 0 {
			out.add("cpu-flags", "node %s lacks cpu flags %s", name, strings.Join(missing, ", "))
		}
	}
}

func (st *preflightState) checkMemory(out *findings) {
	for _, name := range slices.Sorted(maps.Keys(st.in.MemoryMiBByNode)) {
		node, ok := st.nodes[name]
		if !ok {
			continue
		}
		running := 0
		for _, vm := range st.vms {
			if vm.Node == name && vm.Status == "running" {
				running += vm.MaxMemMiB
			}
		}
		planned := st.in.MemoryMiBByNode[name]
		if planned+running+memoryReserveMiB > node.MemTotalMiB {
			out.add("memory",
				"node %s: planned %d MiB + running %d MiB + %d MiB reserve exceeds %d MiB total",
				name, planned, running, memoryReserveMiB, node.MemTotalMiB)
		}
	}
}

func (st *preflightState) checkISOSpace(out *findings) {
	need := 2 * st.in.ISOBytes
	for _, name := range st.in.Nodes {
		inv := st.perNode[name]
		if inv.storagesErr != nil {
			continue
		}
		isISO := func(s StorageInfo) bool { return s.Name == st.in.ISOStorage }
		i := slices.IndexFunc(inv.storages, isISO)
		if i >= 0 && inv.storages[i].AvailBytes < need {
			out.add("iso-space",
				"storage %s on node %s has %d MiB free; need %d MiB (2 x iso size)",
				st.in.ISOStorage, name,
				inv.storages[i].AvailBytes/(1024*1024), need/(1024*1024))
		}
	}
}

func (st *preflightState) checkVMIDCollision(out *findings) {
	for _, vm := range st.vms {
		if slices.Contains(st.in.VMIDs, vm.VMID) {
			out.add("vmid-collision", "vmid %d already used by %q on node %s", vm.VMID, vm.Name, vm.Node)
		}
	}
}

func (st *preflightState) checkMACCollision(ctx context.Context, d Driver, out *findings) {
	planned := make(map[string]string, len(st.in.MACs))
	for _, mac := range st.in.MACs {
		planned[strings.ToLower(mac)] = mac
	}
	for _, vm := range st.vms {
		cfg, err := d.VMConfig(ctx, vm.VMRef)
		if err != nil {
			out.warn("mac-collision", "read config of vmid %d on node %s: %v", vm.VMID, vm.Node, err)
			continue
		}
		if mac, ok := planned[strings.ToLower(cfg.MAC)]; ok && cfg.MAC != "" {
			out.add("mac-collision", "mac %s already used by vmid %d on node %s", mac, vm.VMID, vm.Node)
		}
	}
}

func (st *preflightState) checkAddresses(ctx context.Context, ping Pinger, out *findings) {
	addrs := slices.Clone(st.in.Addresses)
	slices.SortFunc(addrs, netip.Addr.Compare)
	for _, a := range addrs {
		if ping.Alive(ctx, a) {
			out.add("address-in-use", "address %s answers ping", a)
		}
	}
}

func (st *preflightState) checkStorageContent(out *findings) {
	for _, name := range st.in.Nodes {
		inv := st.perNode[name]
		if inv.storagesErr != nil {
			out.add("storage-content", "node %s: list storages: %v", name, inv.storagesErr)
			continue
		}
		checkStorage(out, inv.storages, st.in.Storage, "images", name)
		checkStorage(out, inv.storages, st.in.ISOStorage, "iso", name)
	}
}

func checkStorage(out *findings, storages []StorageInfo, want, content, node string) {
	i := slices.IndexFunc(storages, func(s StorageInfo) bool { return s.Name == want })
	switch {
	case i < 0:
		out.add("storage-content", "storage %s not found on node %s", want, node)
	case !storages[i].Active:
		out.add("storage-content", "storage %s inactive on node %s", want, node)
	case !slices.Contains(storages[i].Content, content):
		out.add("storage-content", "storage %s on node %s lacks %s content", want, node, content)
	}
}

func (st *preflightState) checkBridge(out *findings) {
	for _, name := range st.in.Nodes {
		inv := st.perNode[name]
		if inv.bridgesErr != nil {
			out.add("bridge", "node %s: list bridges: %v", name, inv.bridgesErr)
			continue
		}
		i := slices.IndexFunc(inv.bridges, func(b Bridge) bool { return b.Name == st.in.Bridge })
		switch {
		case i < 0:
			out.add("bridge", "bridge %s not found on node %s", st.in.Bridge, name)
		case !inv.bridges[i].Active:
			out.add("bridge", "bridge %s inactive on node %s", st.in.Bridge, name)
		case st.in.VLAN != 0 && !inv.bridges[i].VLANAware:
			out.add("bridge", "bridge %s on node %s is not vlan-aware; vlan %d needs it",
				st.in.Bridge, name, st.in.VLAN)
		}
	}
}

type execPinger struct {
	ex  *executor.Executor
	bin string
}

// ExecPinger returns a Pinger that runs `ping -c 1 -W <wait> <addr>` through
// ex; a non-zero exit or a missing ping binary both read as not alive.
func ExecPinger(ex *executor.Executor) Pinger {
	return execPinger{ex: ex, bin: "ping"}
}

func (p execPinger) Alive(ctx context.Context, addr netip.Addr) bool {
	res, err := p.ex.RunDiscard(ctx, p.bin, "-c", "1", "-W", pingWait(runtime.GOOS), addr.String())
	return err == nil && res.ExitCode == 0
}

func pingWait(goos string) string {
	if goos == "darwin" {
		return "1000" // macOS ping takes -W in milliseconds, iputils in seconds
	}
	return "1"
}
