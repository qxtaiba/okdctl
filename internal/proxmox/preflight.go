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

	"golang.org/x/sync/errgroup"

	"github.com/qxtaiba/okdctl/internal/executor"
)

// PreflightInput is the planned cluster footprint Preflight checks the
// Proxmox cluster against; NodeAddresses maps each node address to the VMID
// that will hold it, so a running VM of the Tag'd cluster may answer on its own.
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
	NodeAddresses   map[netip.Addr]int
	VIPs            []netip.Addr
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

// Pinger reports whether an address answers on the network; an error means
// the probe was inconclusive (no ping binary, no route), not unanswered.
type Pinger interface {
	Alive(ctx context.Context, addr netip.Addr) (bool, error)
}

// SeverityFail and SeverityWarn are the two Finding.Severity values.
const (
	SeverityFail = "fail"
	SeverityWarn = "warn"

	memoryReserveMiB = 8192
	pingConcurrency  = 8
)

var (
	requiredCPUFlags = []string{"avx2", "bmi2", "fma", "movbe"}
	supportedMajors  = []int{8, 9}
	vmPrivileges     = []string{
		"VM.Allocate", "VM.Config.CDROM", "VM.Config.CPU", "VM.Config.Disk", "VM.Config.HWType",
		"VM.Config.Memory", "VM.Config.Network", "VM.Config.Options", "VM.PowerMgmt", "VM.Audit",
	}
	checkSeverity = map[string]string{
		"permissions": SeverityFail, "version": SeverityFail, "cpu-flags": SeverityFail,
		"memory": SeverityWarn, "iso-space": SeverityFail, "vmid-collision": SeverityFail,
		"mac-collision": SeverityFail, "address-in-use": SeverityFail,
		"storage-content": SeverityFail, "bridge": SeverityFail,
	}
)

type findings []Finding

func (f *findings) add(check, format string, args ...any) {
	f.emit(check, checkSeverity[check], format, args...)
}

func (f *findings) warn(check, format string, args ...any) {
	f.emit(check, SeverityWarn, format, args...)
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
// in check order; it errors only when an inventory call every check needs
// fails or ctx ends, so cancellation never reads as a finding.
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
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
	return ctx.Err()
}

type privReq struct{ path, alt, priv string }

func (st *preflightState) requiredPrivileges() []privReq {
	in := st.in
	var reqs []privReq
	var pool string
	if in.Pool != "" {
		pool = "/pool/" + in.Pool
	}
	for _, p := range vmPrivileges {
		// check_vm_perm accepts a VM privilege on the pool the VM is created into.
		reqs = append(reqs, privReq{"/vms", pool, p})
	}
	sdn := "/sdn/zones/localnetwork/" + in.Bridge
	if in.VLAN != 0 {
		sdn += "/" + strconv.Itoa(in.VLAN) // check_vnet_access checks the tag's own path
	}
	reqs = append(reqs,
		privReq{"/storage/" + in.Storage, "", "Datastore.AllocateSpace"},
		privReq{"/storage/" + in.Storage, "", "Datastore.Audit"},
		privReq{"/storage/" + in.ISOStorage, "", "Datastore.AllocateTemplate"},
		privReq{"/storage/" + in.ISOStorage, "", "Datastore.Allocate"},
		privReq{"/storage/" + in.ISOStorage, "", "Datastore.Audit"},
		privReq{sdn, "", "SDN.Use"},
	)
	for _, n := range in.Nodes {
		reqs = append(reqs, privReq{"/nodes/" + n, "", "Sys.Audit"})
	}
	if in.HA {
		reqs = append(reqs, privReq{"/", "", "Sys.Console"})
	}
	if pool != "" {
		// create_vm with pool= calls check_perm_modify, which wants
		// Permissions.Modify or Pool.Allocate on the pool.
		reqs = append(reqs, privReq{pool, "", "Pool.Allocate"})
	}
	seen := make(map[privReq]bool, len(reqs))
	return slices.DeleteFunc(reqs, func(r privReq) bool {
		dup := seen[r]
		seen[r] = true
		return dup
	})
}

func (st *preflightState) checkPermissions(out *findings) {
	for _, r := range st.requiredPrivileges() {
		if st.perms.granted(r.path, r.priv) || (r.alt != "" && st.perms.granted(r.alt, r.priv)) {
			continue
		}
		where := r.path
		if r.alt != "" {
			where += " or " + r.alt
		}
		out.add("permissions", "missing privilege %s on %s", r.priv, where)
	}
}

// granted follows PVE's roles(): a privilege listed on the path itself is
// held whatever its propagate flag; below a listed path the nearest ancestor
// carrying any propagating entry decides alone, since its roles replace what
// it would inherit, and an ancestor with none is skipped.
func (p Permissions) granted(path, priv string) bool {
	if privs, ok := p[path]; ok {
		_, held := privs[priv]
		return held
	}
	for path != "/" {
		if i := strings.LastIndex(path, "/"); i > 0 {
			path = path[:i]
		} else {
			path = "/"
		}
		if privs := p[path]; slices.Contains(slices.Collect(maps.Values(privs)), true) {
			return privs[priv]
		}
	}
	return false
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
		switch {
		case !ok:
			out.add("cpu-flags", "node %s not in cluster", name)
			continue
		case node.Status != nodeStatusOnline:
			out.add("cpu-flags", "node %s is %s", name, node.Status)
			continue
		case len(node.CPUFlags) == 0:
			out.add("cpu-flags", "node %s: status not readable", name)
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

// owned reports a VM carrying the planned cluster's tag, which a re-run
// meets at its own VMIDs, MACs and memory.
func (st *preflightState) owned(tags []string) bool {
	return st.in.Tag != "" && slices.Contains(tags, st.in.Tag)
}

func (st *preflightState) checkMemory(out *findings) {
	for _, name := range slices.Sorted(maps.Keys(st.in.MemoryMiBByNode)) {
		node, ok := st.nodes[name]
		if !ok || node.MemTotalMiB == 0 {
			continue // an unreadable or offline status is the cpu-flags check's finding
		}
		running, owned := 0, 0
		for i := range st.vms {
			vm := &st.vms[i]
			switch {
			case vm.Node != name || vm.Status != "running":
			case st.owned(vm.Tags):
				owned += vm.MaxMemMiB
			default:
				running += vm.MaxMemMiB
			}
		}
		planned := st.in.MemoryMiBByNode[name]
		// The plan reuses this cluster's own running VMs up to what it places
		// here, so a re-run is not double counted while a VM it moves away
		// still occupies its current node.
		running += max(0, owned-planned)
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
		i := slices.IndexFunc(inv.storages, func(s StorageInfo) bool { return s.Name == st.in.ISOStorage })
		if i < 0 || inv.storages[i].AvailBytes >= need {
			continue
		}
		where := "on node " + name
		if inv.storages[i].Shared {
			where = "(shared)"
		}
		out.add("iso-space", "storage %s %s has %d MiB free; need %d MiB (2 x iso size)",
			st.in.ISOStorage, where, inv.storages[i].AvailBytes/(1024*1024), need/(1024*1024))
		if inv.storages[i].Shared {
			return
		}
	}
}

func (st *preflightState) checkVMIDCollision(out *findings) {
	for i := range st.vms {
		vm := &st.vms[i]
		if !st.owned(vm.Tags) && slices.Contains(st.in.VMIDs, vm.VMID) {
			out.add("vmid-collision", "vmid %d already used by %q on node %s", vm.VMID, vm.Name, vm.Node)
		}
	}
}

func (st *preflightState) checkMACCollision(ctx context.Context, d Driver, out *findings) {
	if len(st.in.MACs) == 0 {
		return
	}
	planned := make(map[string]string, len(st.in.MACs))
	for _, mac := range st.in.MACs {
		planned[strings.ToLower(mac)] = mac
	}
	for i := range st.vms {
		vm := &st.vms[i]
		if ctx.Err() != nil {
			return
		}
		if vm.Type != resourceTypeQEMU || st.owned(vm.Tags) {
			continue
		}
		cfg, err := d.VMConfig(ctx, vm.VMRef)
		if err != nil {
			out.warn("mac-collision", "read config of vmid %d on node %s: %v", vm.VMID, vm.Node, err)
			continue
		}
		for _, have := range cfg.MACs {
			if mac, ok := planned[strings.ToLower(have)]; ok {
				out.add("mac-collision", "mac %s already used by vmid %d on node %s", mac, vm.VMID, vm.Node)
			}
		}
	}
}

func (st *preflightState) checkAddresses(ctx context.Context, ping Pinger, out *findings) {
	if ping == nil {
		return
	}
	addrs := st.unownedAddresses()
	alive := make([]bool, len(addrs))
	errs := make([]error, len(addrs))
	var g errgroup.Group
	g.SetLimit(pingConcurrency)
	for i, a := range addrs {
		g.Go(func() error { alive[i], errs[i] = ping.Alive(ctx, a); return nil })
	}
	_ = g.Wait() // Alive's error is a finding, not a failure, so nothing to collect
	inconclusive := map[string][]string{}
	var causes []string
	for i, a := range addrs {
		switch {
		case errs[i] != nil:
			cause := errs[i].Error()
			if _, seen := inconclusive[cause]; !seen {
				causes = append(causes, cause)
			}
			inconclusive[cause] = append(inconclusive[cause], a.String())
		case alive[i]:
			out.add("address-in-use", "address %s answers ping", a)
		}
	}
	for _, cause := range causes {
		out.warn("address-in-use", "address check inconclusive for %s: %s", strings.Join(inconclusive[cause], ", "), cause)
	}
}

// unownedAddresses drops the addresses the cluster's own running VMs hold:
// a node address while its VM runs, the VIPs while any of them does.
func (st *preflightState) unownedAddresses() []netip.Addr {
	running := map[int]bool{}
	for i := range st.vms {
		vm := &st.vms[i]
		if vm.Status == "running" && st.owned(vm.Tags) {
			running[vm.VMID] = true
		}
	}
	addrs := make([]netip.Addr, 0, len(st.in.NodeAddresses)+len(st.in.VIPs))
	for a, vmid := range st.in.NodeAddresses {
		if !running[vmid] {
			addrs = append(addrs, a)
		}
	}
	if len(running) == 0 {
		addrs = append(addrs, st.in.VIPs...)
	}
	slices.SortFunc(addrs, netip.Addr.Compare)
	return addrs
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
// ex; only the platform's no-reply exit reads as not alive, anything else
// (missing binary, no route, another exit) is inconclusive.
func ExecPinger(ex *executor.Executor) Pinger {
	return execPinger{ex: ex, bin: "ping"}
}

func (p execPinger) Alive(ctx context.Context, addr netip.Addr) (bool, error) {
	wait, noReply := pingFlavour(runtime.GOOS)
	res, err := p.ex.Run(ctx, p.bin, "-c", "1", "-W", wait, addr.String())
	stderr := strings.TrimSpace(res.Stderr)
	switch {
	case err != nil:
		return false, fmt.Errorf("ping: %w", err)
	case res.ExitCode == 0:
		return true, nil
	case res.ExitCode == noReply && !routeError(stderr):
		return false, nil
	}
	return false, fmt.Errorf("ping exit %d: %s", res.ExitCode, stderr)
}

// routeError spots the sendto failures ping reports under the no-reply exit.
func routeError(stderr string) bool {
	return strings.Contains(stderr, "No route to host") || strings.Contains(stderr, "unreachable")
}

// pingFlavour returns the -W value and the no-reply exit code: macOS ping
// takes milliseconds and exits 2, iputils takes seconds and exits 1.
func pingFlavour(goos string) (wait string, noReply int) {
	if goos == "darwin" {
		return "1000", 2
	}
	return "1", 1
}
