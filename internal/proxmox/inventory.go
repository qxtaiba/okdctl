package proxmox

import (
	"context"
	"net/url"
	"slices"
	"strings"

	"github.com/luthermonson/go-proxmox"
)

// num decodes the integers PVE emits as either JSON numbers or strings.
type num = proxmox.StringOrUint64

const (
	nodeStatusOnline = "online"
	resourceTypeQEMU = "qemu"
)

func (d *driver) Version(ctx context.Context) (Version, error) {
	var raw struct {
		Version string `json:"version"`
		Release string `json:"release"`
	}
	if err := d.get(ctx, "/version", &raw); err != nil {
		return Version{}, err
	}
	return Version{Version: raw.Version, Release: raw.Release}, nil
}

func (d *driver) Permissions(ctx context.Context) (Permissions, error) {
	var raw map[string]map[string]num
	if err := d.get(ctx, "/access/permissions", &raw); err != nil {
		return nil, err
	}
	perms := make(Permissions, len(raw))
	for path, privs := range raw {
		granted := make(map[string]bool, len(privs))
		for priv, v := range privs {
			granted[priv] = v != 0
		}
		perms[path] = granted
	}
	return perms, nil
}

type nodeRow struct {
	Node   string `json:"node"`
	Status string `json:"status"`
	MaxMem num    `json:"maxmem"`
	Mem    num    `json:"mem"`
	MaxCPU num    `json:"maxcpu"`
}

type nodeStatus struct {
	PVEVersion string `json:"pveversion"`
	CPUInfo    struct {
		Model string `json:"model"`
		CPUs  num    `json:"cpus"`
		Flags string `json:"flags"`
	} `json:"cpuinfo"`
	Memory struct {
		Total num `json:"total"`
		Used  num `json:"used"`
	} `json:"memory"`
}

// Nodes reads /nodes/{node}/status only for online nodes; an offline node
// keeps the coarser figures the /nodes listing carries.
func (d *driver) Nodes(ctx context.Context) ([]NodeInfo, error) {
	var rows []nodeRow
	if err := d.get(ctx, "/nodes", &rows); err != nil {
		return nil, err
	}
	nodes := make([]NodeInfo, 0, len(rows))
	for _, r := range rows {
		info := NodeInfo{
			Name:        r.Node,
			Status:      r.Status,
			MemTotalMiB: bytesToMiB(uint64(r.MaxMem)),
			MemUsedMiB:  bytesToMiB(uint64(r.Mem)),
			CPUs:        int(r.MaxCPU), //nolint:gosec // G115: CPU counts are small
		}
		if r.Status == nodeStatusOnline {
			var st nodeStatus
			if err := d.get(ctx, nodePath(r.Node, "status"), &st); err != nil {
				return nil, err
			}
			info.MemTotalMiB = bytesToMiB(uint64(st.Memory.Total))
			info.MemUsedMiB = bytesToMiB(uint64(st.Memory.Used))
			info.CPUs = int(st.CPUInfo.CPUs) //nolint:gosec // G115: CPU counts are small
			info.CPUModel = st.CPUInfo.Model
			info.CPUFlags = strings.Fields(st.CPUInfo.Flags)
			info.PVEVersion = st.PVEVersion
		}
		nodes = append(nodes, info)
	}
	slices.SortFunc(nodes, func(a, b NodeInfo) int { return strings.Compare(a.Name, b.Name) })
	return nodes, nil
}

type storageRow struct {
	Storage string `json:"storage"`
	Type    string `json:"type"`
	Shared  num    `json:"shared"`
	Active  num    `json:"active"`
	Content string `json:"content"`
	Avail   num    `json:"avail"`
	Total   num    `json:"total"`
}

func (d *driver) Storages(ctx context.Context, node string) ([]StorageInfo, error) {
	var rows []storageRow
	if err := d.get(ctx, nodePath(node, "storage"), &rows); err != nil {
		return nil, err
	}
	storages := make([]StorageInfo, 0, len(rows))
	for _, r := range rows {
		storages = append(storages, StorageInfo{
			Name:       r.Storage,
			Type:       r.Type,
			Shared:     r.Shared != 0,
			Active:     r.Active != 0,
			Content:    splitList(r.Content, ','),
			AvailBytes: int64(r.Avail), //nolint:gosec // G115: storage sizes fit int64
			TotalBytes: int64(r.Total), //nolint:gosec // G115: storage sizes fit int64
		})
	}
	slices.SortFunc(storages, func(a, b StorageInfo) int { return strings.Compare(a.Name, b.Name) })
	return storages, nil
}

type networkRow struct {
	Iface     string `json:"iface"`
	VLANAware num    `json:"bridge_vlan_aware"`
	Active    num    `json:"active"`
}

func (d *driver) Bridges(ctx context.Context, node string) ([]Bridge, error) {
	var rows []networkRow
	if err := d.get(ctx, nodePath(node, "network")+"?type=any_bridge", &rows); err != nil {
		return nil, err
	}
	bridges := make([]Bridge, 0, len(rows))
	for _, r := range rows {
		bridges = append(bridges, Bridge{Name: r.Iface, VLANAware: r.VLANAware != 0, Active: r.Active != 0})
	}
	slices.SortFunc(bridges, func(a, b Bridge) int { return strings.Compare(a.Name, b.Name) })
	return bridges, nil
}

type resourceRow struct {
	Type   string `json:"type"`
	VMID   num    `json:"vmid"`
	Node   string `json:"node"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Tags   string `json:"tags"`
	Pool   string `json:"pool"`
	MaxMem num    `json:"maxmem"`
	MaxCPU num    `json:"maxcpu"`
}

// ListVMs returns qemu guests only (templates included, containers
// dropped), sorted by VMID, with Tags split on ';' into whole tokens.
func (d *driver) ListVMs(ctx context.Context) ([]VMSummary, error) {
	var rows []resourceRow
	if err := d.get(ctx, "/cluster/resources?type=vm", &rows); err != nil {
		return nil, err
	}
	vms := make([]VMSummary, 0, len(rows))
	for _, r := range rows {
		if r.Type != resourceTypeQEMU {
			continue
		}
		vms = append(vms, VMSummary{
			VMRef:     VMRef{Node: r.Node, VMID: int(r.VMID)}, //nolint:gosec // G115: vmids are small positive integers
			Name:      r.Name,
			Status:    r.Status,
			Tags:      splitList(r.Tags, ';'),
			Pool:      r.Pool,
			MaxMemMiB: bytesToMiB(uint64(r.MaxMem)),
			MaxCPU:    int(r.MaxCPU), //nolint:gosec // G115: CPU counts are small
		})
	}
	slices.SortFunc(vms, func(a, b VMSummary) int { return a.VMID - b.VMID })
	return vms, nil
}

func nodePath(node, rest string) string {
	return "/nodes/" + url.PathEscape(node) + "/" + rest
}

// splitList splits a PVE separator-joined list, dropping empty tokens; a
// nil result for an empty input keeps "no tags" distinct from [""].
func splitList(s string, sep rune) []string {
	if s == "" {
		return nil
	}
	return strings.FieldsFunc(s, func(r rune) bool { return r == sep })
}
