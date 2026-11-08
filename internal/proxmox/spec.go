package proxmox

import (
	"net/url"
	"strconv"
	"strings"
)

// bootDisk is the SCSI slot every okdctl VM boots from once the ISO is detached.
const bootDisk = "scsi0"

// values renders s as the form POST /nodes/{node}/qemu expects.
func (s *VMSpec) values() url.Values {
	v := url.Values{
		"vmid":     {strconv.Itoa(s.VMID)},
		"name":     {s.Name},
		"machine":  {"q35"},
		"bios":     {"ovmf"},
		"efidisk0": {s.EFIStorage + ":1,efitype=4m,pre-enrolled-keys=0"},
		"scsihw":   {"virtio-scsi-single"},
		"net0":     {s.net0()},
		"ide2":     {s.ISOVolID + ",media=cdrom"},
		"boot":     {"order=" + bootDisk + ";ide2"},
		"cpu":      {s.CPUType},
		"cores":    {strconv.Itoa(s.Cores)},
		"sockets":  {"1"},
		"memory":   {strconv.Itoa(s.MemoryMiB)},
		"balloon":  {"0"},
		"agent":    {"enabled=1"},
		"tags":     {s.Tag},
		"onboot":   {"1"},
		"ostype":   {"l26"},
	}
	for _, d := range s.Disks {
		v.Set(d.Slot, d.Storage+":"+strconv.Itoa(d.SizeGiB)+",serial="+d.Serial+",iothread=1,discard=on,cache=none,aio=io_uring")
	}
	if s.Pool != "" {
		v.Set("pool", s.Pool)
	}
	return v
}

func (s *VMSpec) net0() string {
	var b strings.Builder
	b.WriteString("virtio=" + s.MAC + ",bridge=" + s.Bridge)
	if s.VLAN > 0 {
		b.WriteString(",tag=" + strconv.Itoa(s.VLAN))
	}
	return b.String()
}
