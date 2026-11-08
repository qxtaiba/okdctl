package config

import (
	"fmt"
	"maps"
	"net/netip"
	"slices"
	"strings"
)

type roleFloor struct {
	cpu       int
	memoryMiB int
	rootGiB   int
}

func floorFor(role Role) roleFloor {
	if role == RoleMaster {
		return roleFloor{cpu: 4, memoryMiB: 16384, rootGiB: 100}
	}
	return roleFloor{cpu: 2, memoryMiB: 8192, rootGiB: 100}
}

type nodeRef struct {
	name       string
	role       Role
	index      int
	vmid       int
	ip         netip.Addr
	derived    bool
	override   NodeOverride
	overridden bool
}

// validateNodeCount returns the node list the other rules check, or nil
// with a finding when workers.count exceeds MaxWorkers, so no rule derives
// an unbounded list.
func (c *Config) validateNodeCount(r *ValidationResult) []nodeRef {
	switch {
	case c.Workers.Count < 0:
		r.addf(FieldWorkers+".count", "%d must be 0 or more", c.Workers.Count)
	case c.Workers.Count > MaxWorkers:
		r.addf(FieldWorkers+".count", "%d exceeds the maximum of %d", c.Workers.Count, MaxWorkers)
		return nil
	}
	return c.nodes()
}

func (c *Config) validateNodes(r *ValidationResult, nodes []nodeRef) {
	validateDefaults(r, FieldControlPlane, &c.ControlPlane, floorFor(RoleMaster))
	if c.Workers.Count > 0 {
		validateDefaults(r, FieldWorkers, &c.Workers.NodeDefaults, floorFor(RoleWorker))
	}
	c.validateOverrideKeys(r)
	for i := range nodes {
		c.validateOverride(r, nodes, i)
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
		role, _, ok := c.ParseNodeKey(key)
		if !ok {
			r.addf(FieldNodes+"."+key, "not a node name this config produces; keys match %s-master<0..%d> or %s-worker<i>",
				c.Cluster.Name, MasterCount-1, c.Cluster.Name)
			continue
		}
		if role == RoleWorker {
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
	if pool := c.Provider.Proxmox.Nodes; len(pool) > 0 && o.Host != "" && !slices.Contains(pool, o.Host) {
		r.addf(field+".host", "%s host %q is not one of provider.proxmox.nodes (%s)", node.name, o.Host, strings.Join(pool, ", "))
	}
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

// nodes lists the config's nodes masters first, then workers by index, so
// the last entry carries the highest VMID and derived address.
func (c *Config) nodes() []nodeRef {
	workers := c.WorkerIndices()
	refs := make([]nodeRef, 0, MasterCount+len(workers))
	for i := range MasterCount {
		refs = append(refs, c.nodeRef(RoleMaster, i))
	}
	for _, i := range workers {
		refs = append(refs, c.nodeRef(RoleWorker, i))
	}
	return refs
}

func (c *Config) nodeRef(role Role, index int) nodeRef {
	name := c.NodeName(role, index)
	override, overridden := c.Nodes[name]
	ref := nodeRef{name: name, role: role, index: index, vmid: c.NodeVMID(role, index), override: override, overridden: overridden}
	if override.IP != "" {
		ref.ip, _ = netip.ParseAddr(override.IP)
		return ref
	}
	if addr, err := c.DerivedNodeAddress(role, index); err == nil {
		ref.ip, ref.derived = addr, true
	}
	return ref
}
