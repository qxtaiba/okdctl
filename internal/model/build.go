package model

import (
	"errors"
	"fmt"
	"net/netip"

	"github.com/qxtaiba/okdctl/internal/config"
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
	if cfg.Workers.Count < 0 {
		return Inventory{}, fmt.Errorf("workers.count %d must be 0 or more", cfg.Workers.Count)
	}
	if cfg.Workers.Count > config.MaxWorkers {
		return Inventory{}, fmt.Errorf("workers.count %d exceeds the maximum of %d", cfg.Workers.Count, config.MaxWorkers)
	}
	b := &builder{cfg: cfg, infraID: infraID, cidr: cidr, hosts: cfg.Provider.Proxmox.Nodes}

	inv := Inventory{InfraID: infraID, APIVIP: apiVIP, IngressVIP: ingressVIP}
	for i := range config.MasterCount {
		node, err := b.node(RoleMaster, i)
		if err != nil {
			return Inventory{}, err
		}
		inv.Nodes = append(inv.Nodes, node)
	}
	for _, i := range cfg.WorkerIndices() {
		node, err := b.node(RoleWorker, i)
		if err != nil {
			return Inventory{}, err
		}
		inv.Nodes = append(inv.Nodes, node)
	}
	return inv, nil
}

// node places hosts by role and index, never by list position, so a
// refilled worker slot cannot move an existing VM.
func (b *builder) node(role Role, index int) (NodeSpec, error) {
	name := b.cfg.NodeName(role, index)
	override := b.cfg.Nodes[name]
	defaults, hostSlot := b.cfg.ControlPlane, index
	if role == RoleWorker {
		defaults, hostSlot = b.cfg.Workers.NodeDefaults, config.MasterCount+index
	}

	vmid := b.cfg.NodeVMID(role, index)
	if vmid > config.MaxVMID {
		return NodeSpec{}, fmt.Errorf("node %s: vmid %d exceeds 24 bits", name, vmid)
	}
	ip, err := b.address(name, role, index, override.IP)
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

func (b *builder) address(name string, role Role, index int, override string) (netip.Addr, error) {
	var ip netip.Addr
	var err error
	if override != "" {
		if ip, err = netip.ParseAddr(override); err != nil {
			return netip.Addr{}, fmt.Errorf("node %s: parse address: %w", name, err)
		}
	} else if ip, err = b.cfg.DerivedNodeAddress(role, index); err != nil {
		return netip.Addr{}, fmt.Errorf("node %s: derive address: %w", name, err)
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
