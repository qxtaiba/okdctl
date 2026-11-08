package model

import (
	"net/netip"
	"strings"

	"github.com/qxtaiba/okdctl/internal/config"
)

// Role is a node's cluster role, config's layout vocabulary.
type Role = config.Role

// Roles the inventory carries.
const (
	RoleMaster = config.RoleMaster
	RoleWorker = config.RoleWorker
)

// Disk is one virtual disk with its storage and serial resolved.
type Disk struct {
	SizeGiB int    `json:"size_gib"`
	Storage string `json:"storage"`
	Serial  string `json:"serial"`
	Wipe    bool   `json:"wipe"`
}

// NodeSpec is the fully derived identity and sizing of one node.
type NodeSpec struct {
	Role      Role       `json:"role"`
	Index     int        `json:"index"`
	Name      string     `json:"name"`
	VMID      int        `json:"vmid"`
	MAC       string     `json:"mac"`
	IP        netip.Addr `json:"ip"`
	Host      string     `json:"host"`
	CPU       int        `json:"cpu"`
	MemoryMiB int        `json:"memory_mib"`
	Disks     []Disk     `json:"disks"`
}

// Inventory is every node of a cluster, masters first, then workers by index.
type Inventory struct {
	InfraID    string     `json:"infra_id"`
	APIVIP     netip.Addr `json:"api_vip"`
	IngressVIP netip.Addr `json:"ingress_vip"`
	Nodes      []NodeSpec `json:"nodes"`
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
