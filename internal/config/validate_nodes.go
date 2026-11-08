package config

import (
	"fmt"
	"maps"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	"github.com/qxtaiba/okdctl/internal/netutil"
)

// The layout constants mirror internal/model, which derives the same names,
// VMIDs and addresses; model imports config, so the values live here.
const (
	masterCount      = 3
	masterVMIDOffset = 10
	workerVMIDOffset = 100
	workerIPOffset   = 10
	maxVMID          = 0xFFFFFF
	maxDiskSerialLen = 20

	roleMaster = "master"
	roleWorker = "worker"
)

type roleFloor struct {
	cpu       int
	memoryMiB int
	rootGiB   int
}

func floorFor(role string) roleFloor {
	if role == roleMaster {
		return roleFloor{cpu: 4, memoryMiB: 16384, rootGiB: 100}
	}
	return roleFloor{cpu: 2, memoryMiB: 8192, rootGiB: 100}
}

type nodeRef struct {
	name       string
	role       string
	index      int
	vmid       int
	ip         netip.Addr
	derived    bool
	override   NodeOverride
	overridden bool
}

func (c *Config) validateNodes(r *ValidationResult) {
	validateDefaults(r, FieldControlPlane, &c.ControlPlane, floorFor(roleMaster))
	if c.Workers.Count < 0 {
		r.addf(FieldWorkers+".count", "%d must be 0 or more", c.Workers.Count)
	}
	if c.Workers.Count > 0 {
		validateDefaults(r, FieldWorkers, &c.Workers.NodeDefaults, floorFor(roleWorker))
	}
	c.validateOverrideKeys(r)
	nodes := c.nodes()
	for i := range nodes {
		c.validateOverride(r, nodes, i)
		c.validateSerials(r, &nodes[i])
	}
}

func validateDefaults(r *ValidationResult, field string, d *NodeDefaults, f roleFloor) {
	if d.CPU < f.cpu {
		r.addf(field+".cpu", "%d must be at least %d vCPU", d.CPU, f.cpu)
	}
	if d.MemoryMiB < f.memoryMiB {
		r.addf(field+".memory_mib", "%d must be at least %d MiB", d.MemoryMiB, f.memoryMiB)
	}
	if len(d.Disks) == 0 {
		r.AddError(field+".disks", "list at least one disk; disks[0] is the root disk")
	}
	validateDisks(r, field, d.Disks, f.rootGiB)
}

func validateDisks(r *ValidationResult, field string, disks []DiskSpec, rootGiB int) {
	for n, d := range disks {
		sizeField := fmt.Sprintf("%s.disks[%d].size_gib", field, n)
		switch {
		case d.SizeGiB <= 0:
			r.addf(sizeField, "%d must be greater than 0", d.SizeGiB)
		case n == 0 && d.SizeGiB < rootGiB:
			r.addf(sizeField, "root disk %d must be at least %d GiB", d.SizeGiB, rootGiB)
		}
	}
}

func (c *Config) validateOverrideKeys(r *ValidationResult) {
	workers := 0
	for _, key := range slices.Sorted(maps.Keys(c.Nodes)) {
		role, _, ok := c.parseNodeKey(key)
		if !ok {
			r.addf(FieldNodes+"."+key, "not a node name this config produces; keys match %s-master<0..%d> or %s-worker<i>",
				c.Cluster.Name, masterCount-1, c.Cluster.Name)
			continue
		}
		if role == roleWorker {
			workers++
		}
	}
	if workers > c.Workers.Count {
		r.addf(FieldNodes, "%d worker overrides exceed workers.count %d", workers, c.Workers.Count)
	}
}

func (c *Config) validateOverride(r *ValidationResult, nodes []nodeRef, i int) {
	node := &nodes[i]
	if !node.overridden {
		return
	}
	field := FieldNodes + "." + node.name
	f := floorFor(node.role)
	o := &node.override
	if o.CPU != nil && *o.CPU < f.cpu {
		r.addf(field+".cpu", "%d must be at least %d vCPU", *o.CPU, f.cpu)
	}
	if o.MemoryMiB != nil && *o.MemoryMiB < f.memoryMiB {
		r.addf(field+".memory_mib", "%d must be at least %d MiB", *o.MemoryMiB, f.memoryMiB)
	}
	validateDisks(r, field, o.Disks, f.rootGiB)
	if o.IP != "" {
		c.validateOverrideIP(r, nodes, i)
	}
}

// validateOverrideIP reports a shared address once, from the later of two
// overriding nodes, and names both.
func (c *Config) validateOverrideIP(r *ValidationResult, nodes []nodeRef, i int) {
	node := &nodes[i]
	field := FieldNodes + "." + node.name + ".ip"
	ip, err := netip.ParseAddr(node.override.IP)
	if err != nil {
		r.addf(field, "%q is not an IP address", node.override.IP)
		return
	}
	if machine, err := netip.ParsePrefix(c.Networking.MachineCIDR); err == nil && !machine.Contains(ip) {
		r.addf(field, "%s is outside machine_cidr %s", ip, machine.Masked())
	}
	for j := range nodes {
		other := &nodes[j]
		if j == i || (j > i && other.override.IP != "") {
			continue
		}
		if other.ip == ip {
			r.addf(field, "%s and %s share %s", node.name, other.name, ip)
		}
	}
	n := &c.Networking
	for _, peer := range []struct{ name, value string }{
		{"api_vip", n.APIVIP}, {"ingress_vip", n.IngressVIP}, {"gateway", n.Gateway},
	} {
		if addr, err := netip.ParseAddr(peer.value); err == nil && addr == ip {
			r.addf(field, "%s and %s share %s", node.name, peer.name, ip)
		}
	}
	for k, d := range n.DNS {
		if addr, err := netip.ParseAddr(d); err == nil && addr == ip {
			r.addf(field, "%s and dns[%d] share %s", node.name, k, ip)
		}
	}
}

func (c *Config) validateSerials(r *ValidationResult, node *nodeRef) {
	disks := c.ControlPlane.Disks
	if node.role == roleWorker {
		disks = c.Workers.Disks
	}
	if len(node.override.Disks) > 0 {
		disks = node.override.Disks
	}
	for n := range disks {
		if serial := fmt.Sprintf("okd%dd%d", node.vmid, n); len(serial) > maxDiskSerialLen {
			r.addf(fmt.Sprintf("%s.%s.disks[%d]", FieldNodes, node.name, n), "serial %s exceeds %d characters", serial, maxDiskSerialLen)
		}
	}
}

func (c *Config) nodes() []nodeRef {
	refs := make([]nodeRef, 0, masterCount+max(c.Workers.Count, 0))
	for i := range masterCount {
		refs = append(refs, c.nodeRef(roleMaster, i))
	}
	for _, i := range c.workerIndices() {
		refs = append(refs, c.nodeRef(roleWorker, i))
	}
	return refs
}

func (c *Config) nodeRef(role string, index int) nodeRef {
	name := fmt.Sprintf("%s-%s%d", c.Cluster.Name, role, index)
	override, overridden := c.Nodes[name]
	vmidOffset, ipOffset := masterVMIDOffset, 0
	if role == roleWorker {
		vmidOffset, ipOffset = workerVMIDOffset, workerIPOffset
	}
	ref := nodeRef{name: name, role: role, index: index, vmid: c.Provider.Proxmox.VMIDBase + vmidOffset + index, override: override, overridden: overridden}
	if override.IP != "" {
		ref.ip, _ = netip.ParseAddr(override.IP)
		return ref
	}
	if raw, err := netutil.CalculateVMIP(c.Networking.NodeIPStart, ipOffset+index); err == nil {
		ref.ip, ref.derived = netip.MustParseAddr(raw), true
	}
	return ref
}

// parseNodeKey accepts exactly the names nodeRef produces: masters 0..2 and
// any non-negative worker index without leading zeros.
func (c *Config) parseNodeKey(key string) (role string, index int, ok bool) {
	rest, found := strings.CutPrefix(key, c.Cluster.Name+"-")
	if !found {
		return "", 0, false
	}
	for _, role := range []string{roleMaster, roleWorker} {
		digits, found := strings.CutPrefix(rest, role)
		if !found {
			continue
		}
		i, err := strconv.Atoi(digits)
		if err != nil || i < 0 || strconv.Itoa(i) != digits || (role == roleMaster && i >= masterCount) {
			return "", 0, false
		}
		return role, i, true
	}
	return "", 0, false
}

// workerIndices mirrors model's slot filling: explicit nodes: indices first,
// then the lowest free indices up to workers.count.
func (c *Config) workerIndices() []int {
	count := c.Workers.Count
	var explicit []int
	for key := range c.Nodes {
		if role, i, ok := c.parseNodeKey(key); ok && role == roleWorker {
			explicit = append(explicit, i)
		}
	}
	slices.Sort(explicit)
	if len(explicit) >= count {
		return explicit[:max(count, 0)]
	}
	indices := explicit
	for next := 0; len(indices) < count; next++ {
		if !slices.Contains(explicit, next) {
			indices = append(indices, next)
		}
	}
	slices.Sort(indices)
	return indices
}

func (c *Config) highestWorkerIndex() int {
	indices := c.workerIndices()
	if len(indices) == 0 {
		return -1
	}
	return indices[len(indices)-1]
}

func (c *Config) highestVMID() int {
	if hw := c.highestWorkerIndex(); hw >= 0 {
		return c.Provider.Proxmox.VMIDBase + workerVMIDOffset + hw
	}
	return c.Provider.Proxmox.VMIDBase + masterVMIDOffset + masterCount - 1
}

func (c *Config) highestIPOffset() int {
	if hw := c.highestWorkerIndex(); hw >= 0 {
		return workerIPOffset + hw
	}
	return masterCount - 1
}
