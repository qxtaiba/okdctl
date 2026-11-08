package proxmox

import (
	"net/url"
	"strconv"
	"strings"
)

// The fixed shape every okdctl VM is created with; Diff holds configs to it.
const (
	bootDisk   = "scsi0"
	cdromSlot  = "ide2"
	vmMachine  = "q35"
	vmBIOS     = "ovmf"
	vmSCSIHW   = "virtio-scsi-single"
	vmOSType   = "l26"
	vmNICModel = "virtio"
	vmSockets  = 1
	vmBalloon  = 0
)

// values renders s as the form POST /nodes/{node}/qemu expects.
func (s *VMSpec) values() url.Values {
	v := url.Values{
		"vmid":     {strconv.Itoa(s.VMID)},
		"name":     {s.Name},
		"machine":  {vmMachine},
		"bios":     {vmBIOS},
		"efidisk0": {s.EFIStorage + ":1,efitype=4m,pre-enrolled-keys=0"},
		"scsihw":   {vmSCSIHW},
		"net0":     {s.net0()},
		cdromSlot:  {s.ISOVolID + ",media=cdrom"},
		"boot":     {s.bootOrder()},
		"cpu":      {s.CPUType},
		"cores":    {strconv.Itoa(s.Cores)},
		"sockets":  {strconv.Itoa(vmSockets)},
		"memory":   {strconv.Itoa(s.MemoryMiB)},
		"balloon":  {strconv.Itoa(vmBalloon)},
		"agent":    {"enabled=1"},
		"tags":     {s.Tag},
		"onboot":   {"1"},
		"ostype":   {vmOSType},
	}
	for _, d := range s.Disks {
		v.Set(d.Slot, d.Storage+":"+strconv.Itoa(d.SizeGiB)+",serial="+d.Serial+",iothread=1,discard=on,cache=none,aio=io_uring")
	}
	if s.Pool != "" {
		v.Set("pool", s.Pool)
	}
	return v
}

// bootOrder lists the CD-ROM only while the spec still mounts an ISO.
func (s *VMSpec) bootOrder() string {
	if s.ISOVolID == "" {
		return "order=" + bootDisk
	}
	return "order=" + bootDisk + ";" + cdromSlot
}

func (s *VMSpec) net0() string {
	var b strings.Builder
	b.WriteString(vmNICModel + "=" + s.MAC + ",bridge=" + s.Bridge)
	if s.VLAN > 0 {
		b.WriteString(",tag=" + strconv.Itoa(s.VLAN))
	}
	return b.String()
}
