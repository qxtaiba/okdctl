package model

import (
	"net/netip"
	"strings"
)

// Role is a node's cluster role.
type Role string

// Role values match the openshift-install and ignition vocabulary verbatim.
const (
	RoleMaster Role = "master"
	RoleWorker Role = "worker"
)

// Disk is one virtual disk with its storage and serial resolved.
type Disk struct {
	SizeGiB int
	Storage string
	Serial  string
	Wipe    bool
}

// NodeSpec is the fully derived identity and sizing of one node.
type NodeSpec struct {
	Role      Role
	Index     int
	Name      string
	VMID      int
	MAC       string
	IP        netip.Addr
	Host      string
	CPU       int
	MemoryMiB int
	Disks     []Disk
}

// Inventory is every node of a cluster, masters first, then workers by index.
type Inventory struct {
	InfraID    string
	Nodes      []NodeSpec
	APIVIP     netip.Addr
	IngressVIP netip.Addr
}

// ByName returns the node called name.
func (inv *Inventory) ByName(name string) (NodeSpec, bool) {
	return inv.find(func(n *NodeSpec) bool { return n.Name == name })
}

// ByVMID returns the node with VMID id.
func (inv *Inventory) ByVMID(id int) (NodeSpec, bool) {
	return inv.find(func(n *NodeSpec) bool { return n.VMID == id })
}

// ByMAC returns the node with the given MAC, compared case-insensitively.
func (inv *Inventory) ByMAC(mac string) (NodeSpec, bool) {
	return inv.find(func(n *NodeSpec) bool { return strings.EqualFold(n.MAC, mac) })
}

// Masters returns the control-plane nodes in index order.
func (inv *Inventory) Masters() []NodeSpec {
	return inv.withRole(RoleMaster)
}

// Workers returns the worker nodes in index order.
func (inv *Inventory) Workers() []NodeSpec {
	return inv.withRole(RoleWorker)
}

func (inv *Inventory) find(match func(*NodeSpec) bool) (NodeSpec, bool) {
	for i := range inv.Nodes {
		if n := &inv.Nodes[i]; match(n) {
			return *n, true
		}
	}
	return NodeSpec{}, false
}

func (inv *Inventory) withRole(role Role) []NodeSpec {
	var out []NodeSpec
	for i := range inv.Nodes {
		if inv.Nodes[i].Role == role {
			out = append(out, inv.Nodes[i])
		}
	}
	return out
}
