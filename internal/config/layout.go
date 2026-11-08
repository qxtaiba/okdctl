package config

import (
	"fmt"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	"github.com/qxtaiba/okdctl/internal/netutil"
)

// Role is a node's cluster role, in openshift-install vocabulary.
type Role string

// Roles the layout produces.
const (
	RoleMaster Role = "master"
	RoleWorker Role = "worker"
)

// MasterCount and the offsets place <cluster>-master<i> at vmid_base+10+i
// and node_ip_start+i, <cluster>-worker<i> at vmid_base+100+i and node_ip_start+10+i.
const (
	MasterCount      = 3
	MasterVMIDOffset = 10
	WorkerVMIDOffset = 100
	MasterIPOffset   = 0
	WorkerIPOffset   = 10
	MaxVMID          = 0xFFFFFF
)

// NodeName returns the name of node index of role.
func (c *Config) NodeName(role Role, index int) string {
	return fmt.Sprintf("%s-%s%d", c.Cluster.Name, role, index)
}

// NodeVMID returns the VMID of node index of role.
func (c *Config) NodeVMID(role Role, index int) int {
	if role == RoleWorker {
		return c.Provider.Proxmox.VMIDBase + WorkerVMIDOffset + index
	}
	return c.Provider.Proxmox.VMIDBase + MasterVMIDOffset + index
}

// NodeIPOffset returns the distance of node index of role from node_ip_start.
func NodeIPOffset(role Role, index int) int {
	if role == RoleWorker {
		return WorkerIPOffset + index
	}
	return MasterIPOffset + index
}

// DerivedNodeAddress returns the address node_ip_start gives node index of
// role, before any nodes.<name>.ip override.
func (c *Config) DerivedNodeAddress(role Role, index int) (netip.Addr, error) {
	raw, err := netutil.CalculateVMIP(c.Networking.NodeIPStart, NodeIPOffset(role, index))
	if err != nil {
		return netip.Addr{}, err
	}
	return netip.ParseAddr(raw)
}

// ParseNodeKey accepts exactly the names NodeName produces: masters 0..2
// and any non-negative worker index without leading zeros.
func (c *Config) ParseNodeKey(key string) (role Role, index int, ok bool) {
	rest, found := strings.CutPrefix(key, c.Cluster.Name+"-")
	if !found {
		return "", 0, false
	}
	for _, role := range []Role{RoleMaster, RoleWorker} {
		digits, found := strings.CutPrefix(rest, string(role))
		if !found {
			continue
		}
		i, err := strconv.Atoi(digits)
		if err != nil || i < 0 || strconv.Itoa(i) != digits || (role == RoleMaster && i >= MasterCount) {
			return "", 0, false
		}
		return role, i, true
	}
	return "", 0, false
}

// WorkerIndices lists the worker indices this config produces, in order: the
// ones named under nodes first, so a removed worker's slot is the one a later
// add reuses, then the lowest free ones up to workers.count.
func (c *Config) WorkerIndices() []int {
	count := max(c.Workers.Count, 0)
	var explicit []int
	for key := range c.Nodes {
		if role, i, ok := c.ParseNodeKey(key); ok && role == RoleWorker {
			explicit = append(explicit, i)
		}
	}
	slices.Sort(explicit)
	if len(explicit) >= count {
		return explicit[:count]
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

// lastNode returns the node with the highest VMID and address: the top
// worker, or master2 when there are no workers.
func (c *Config) lastNode() (role Role, index int) {
	if indices := c.WorkerIndices(); len(indices) > 0 {
		return RoleWorker, indices[len(indices)-1]
	}
	return RoleMaster, MasterCount - 1
}
