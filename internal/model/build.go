package model

import (
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/netutil"
)

const (
	masterCount      = 3
	masterVMIDOffset = 10
	workerVMIDOffset = 100
	workerIPOffset   = 10
	maxVMID          = 0xFFFFFF
)

type builder struct {
	cfg     *config.Config
	infraID string
	cidr    netip.Prefix
	hosts   []string
}

// Build derives the Inventory of cfg; the result is deterministic for a
// given config and infraID, so existing nodes keep their identity across
// worker additions and removals.
func Build(cfg *config.Config, infraID string) (Inventory, error) {
	cidr, err := netip.ParsePrefix(cfg.Networking.MachineCIDR)
	if err != nil {
		return Inventory{}, fmt.Errorf("parse machine_cidr: %w", err)
	}
	apiVIP, err := netip.ParseAddr(cfg.Networking.APIVIP)
	if err != nil {
		return Inventory{}, fmt.Errorf("parse api_vip: %w", err)
	}
	ingressVIP, err := netip.ParseAddr(cfg.Networking.IngressVIP)
	if err != nil {
		return Inventory{}, fmt.Errorf("parse ingress_vip: %w", err)
	}
	if len(cfg.Provider.Proxmox.Nodes) == 0 {
		return Inventory{}, errors.New("provider.proxmox.nodes is empty")
	}
	b := &builder{cfg: cfg, infraID: infraID, cidr: cidr, hosts: cfg.Provider.Proxmox.Nodes}

	inv := Inventory{InfraID: infraID, APIVIP: apiVIP, IngressVIP: ingressVIP}
	for i := range masterCount {
		node, err := b.node(RoleMaster, i)
		if err != nil {
			return Inventory{}, err
		}
		inv.Nodes = append(inv.Nodes, node)
	}
	for _, i := range b.workerIndices() {
		node, err := b.node(RoleWorker, i)
		if err != nil {
			return Inventory{}, err
		}
		inv.Nodes = append(inv.Nodes, node)
	}
	return inv, nil
}

// workerIndices keeps the indices named under nodes: so a removed worker's
// slot is the one a later add reuses, then fills from zero up to count.
func (b *builder) workerIndices() []int {
	count := b.cfg.Workers.Count
	prefix := b.cfg.Cluster.Name + "-" + string(RoleWorker)
	var explicit []int
	for name := range b.cfg.Nodes {
		rest, ok := strings.CutPrefix(name, prefix)
		if !ok {
			continue
		}
		if i, err := strconv.Atoi(rest); err == nil && i >= 0 && strconv.Itoa(i) == rest {
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

// node places hosts by role and index, never by list position, so a
// refilled worker slot cannot move an existing VM.
func (b *builder) node(role Role, index int) (NodeSpec, error) {
	name := fmt.Sprintf("%s-%s%d", b.cfg.Cluster.Name, role, index)
	override := b.cfg.Nodes[name]
	defaults, vmidOffset, ipOffset, hostSlot := b.cfg.ControlPlane, masterVMIDOffset, 0, index
	if role == RoleWorker {
		defaults, vmidOffset, ipOffset, hostSlot = b.cfg.Workers.NodeDefaults, workerVMIDOffset, workerIPOffset, masterCount+index
	}

	vmid := b.cfg.Provider.Proxmox.VMIDBase + vmidOffset + index
	if vmid > maxVMID {
		return NodeSpec{}, fmt.Errorf("node %s: vmid %d exceeds 24 bits", name, vmid)
	}
	ip, err := b.address(name, override.IP, ipOffset+index)
	if err != nil {
		return NodeSpec{}, err
	}

	spec := NodeSpec{
		Role: role, Index: index, Name: name, VMID: vmid, MAC: MAC(b.infraID, vmid), IP: ip,
		Host: b.hosts[hostSlot%len(b.hosts)], CPU: defaults.CPU, MemoryMiB: defaults.MemoryMiB,
	}
	if override.Host != "" {
		spec.Host = override.Host
	}
	if override.CPU != nil {
		spec.CPU = *override.CPU
	}
	if override.MemoryMiB != nil {
		spec.MemoryMiB = *override.MemoryMiB
	}
	disks := defaults.Disks
	if len(override.Disks) > 0 {
		disks = override.Disks
	}
	spec.Disks = b.disks(vmid, disks)
	return spec, nil
}

func (b *builder) address(name, override string, offset int) (netip.Addr, error) {
	raw := override
	if raw == "" {
		var err error
		if raw, err = netutil.CalculateVMIP(b.cfg.Networking.NodeIPStart, offset); err != nil {
			return netip.Addr{}, fmt.Errorf("node %s: derive address: %w", name, err)
		}
	}
	ip, err := netip.ParseAddr(raw)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("node %s: parse address: %w", name, err)
	}
	if !b.cidr.Contains(ip) {
		return netip.Addr{}, fmt.Errorf("node %s: address %s outside machine_cidr %s", name, ip, b.cidr)
	}
	return ip, nil
}

func (b *builder) disks(vmid int, specs []config.DiskSpec) []Disk {
	disks := make([]Disk, 0, len(specs))
	for n, d := range specs {
		disk := Disk{SizeGiB: d.SizeGiB, Storage: d.Storage, Serial: DiskSerial(vmid, n)}
		if disk.Storage == "" {
			disk.Storage = b.cfg.Provider.Proxmox.Storage
		}
		if d.Wipe != nil {
			disk.Wipe = *d.Wipe
		}
		disks = append(disks, disk)
	}
	return disks
}
