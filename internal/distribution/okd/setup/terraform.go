package setup

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/templates"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/system"
)

// DefaultProxmoxCPUType is the Proxmox qemu cpu type used when the operator
// has not set proxmox.cpuType. "host" passes every CPU flag through so nested
// virt and vector extensions (AVX2, AES-NI) required by OKD nodes work.
const DefaultProxmoxCPUType = "host"

func buildQuotedRoleList(format, prefix string, role nodetypes.NodeRole, count int) []string {
	result := make([]string, count)
	for i := range count {
		result[i] = fmt.Sprintf(format, prefix, role, i)
	}
	return result
}

func buildISOStrings(isoStorage string, role nodetypes.NodeRole, count int) []string {
	return buildQuotedRoleList(`"%s:iso/%s%d.iso"`, isoStorage, role, count)
}

func buildNodeNames(clusterName string, role nodetypes.NodeRole, count int) []string {
	return buildQuotedRoleList(`"%s-%s%d"`, clusterName, role, count)
}

type diskSizes struct {
	cpOS, workerOS, workerData, cpData int
}

func getDiskSizes(cfg *config.Config) diskSizes {
	d := diskSizes{
		cpOS:       cfg.Topology.ControlPlane.DiskGB,
		workerData: cfg.Disks.WorkerDataSizeGB,
		cpData:     cfg.Disks.ControlPlaneDataSizeGB,
	}
	if d.cpOS == 0 {
		d.cpOS = 50
	}
	d.workerOS = cfg.Topology.Workers.DiskGB
	if d.workerOS == 0 {
		d.workerOS = d.cpOS
	}
	return d
}

func getBootstrapResources(cfg *config.Config) (cpu, mem int) {
	cpu = cfg.Topology.Bootstrap.CPU
	mem = cfg.Topology.Bootstrap.MemoryMB
	if cpu == 0 {
		cpu = cfg.Topology.ControlPlane.CPU
	}
	if mem == 0 {
		mem = cfg.Topology.ControlPlane.MemoryMB
	}
	return cpu, mem
}

func buildTerraformVarsData(cfg *config.Config) templates.TerraformVarsData {
	proxmox := cfg.Provider.Proxmox
	disks := getDiskSizes(cfg)
	bootstrapCPU, bootstrapMem := getBootstrapResources(cfg)

	masterISOs := buildISOStrings(proxmox.ISOStorage, nodetypes.RoleMaster, cfg.Topology.ControlPlane.Count)
	workerISOs := buildISOStrings(proxmox.ISOStorage, nodetypes.RoleWorker, cfg.Topology.Workers.Count)
	masterNames := buildNodeNames(cfg.Cluster.Name, nodetypes.RoleMaster, cfg.Topology.ControlPlane.Count)
	workerNames := buildNodeNames(cfg.Cluster.Name, nodetypes.RoleWorker, cfg.Topology.Workers.Count)

	cpuType := proxmox.CPUType
	if cpuType == "" {
		cpuType = DefaultProxmoxCPUType
	}

	return templates.TerraformVarsData{
		ClusterName:          cfg.Cluster.Name,
		TargetNode:           proxmox.Node,
		Bridge:               proxmox.Bridge,
		OSStorage:            proxmox.Storage,
		DataStorage:          proxmox.DataStorage,
		FCOSISOStorage:       proxmox.ISOStorage,
		MasterISOsString:     strings.Join(masterISOs, ", "),
		WorkerISOsString:     strings.Join(workerISOs, ", "),
		VMIDBase:             cfg.Topology.VMIDBase,
		MasterCount:          cfg.Topology.ControlPlane.Count,
		WorkerCount:          cfg.Topology.Workers.Count,
		OSDiskSizeGB:         disks.cpOS,
		MasterOSDiskSizeGB:   disks.cpOS,
		WorkerOSDiskSizeGB:   disks.workerOS,
		WorkerDataDiskSizeGB: disks.workerData,
		MasterDataDiskSizeGB: disks.cpData,
		BootstrapCPUCores:    bootstrapCPU,
		BootstrapMemoryMB:    bootstrapMem,
		MasterCPUCores:       cfg.Topology.ControlPlane.CPU,
		MasterMemoryMB:       cfg.Topology.ControlPlane.MemoryMB,
		WorkerCPUCores:       cfg.Topology.Workers.CPU,
		WorkerMemoryMB:       cfg.Topology.Workers.MemoryMB,
		MasterNames:          strings.Join(masterNames, ", "),
		WorkerNames:          strings.Join(workerNames, ", "),
		CPUType:              cpuType,
		NUMAEnabled:          proxmox.NUMAEnabled,
		AdditionalNetworks:   formatAdditionalNetworks(proxmox.AdditionalNetworks),
		MasterTargetNodes:    formatStringList(proxmox.ControlPlaneNodes),
		WorkerTargetNodes:    formatStringList(proxmox.WorkerNodes),
	}
}

func formatStringList(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = fmt.Sprintf("%q", item)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func formatAdditionalNetworks(networks []config.AdditionalNetwork) string {
	if len(networks) == 0 {
		return "[]"
	}
	var parts []string
	for _, n := range networks {
		model := n.Model
		if model == "" {
			model = "virtio"
		}
		entry := fmt.Sprintf(`{ model = %q, bridge = %q`, model, n.Bridge)
		if n.VLANTag > 0 {
			entry += fmt.Sprintf(`, tag = %d`, n.VLANTag)
		}
		entry += " }"
		parts = append(parts, entry)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// GenerateTerraformVars renders terraform.tfvars for the Proxmox provider
// into the environment directory derived from cfg.
func (p *Phase) GenerateTerraformVars(ctx context.Context, cfg *config.Config, opts *Options) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if cfg.Provider.Proxmox == nil {
		return &errtypes.ConfigError{Msg: "proxmox provider configuration required"}
	}

	data := buildTerraformVarsData(cfg)
	envDir := filepath.Join(opts.ProjectRoot, "infrastructure", "terraform", "environments", phase.GetTerraformEnv(cfg))
	// A stale postinstall sentinel would override the regenerated
	// bootstrap_enabled=true and silently skip the bootstrap VM.
	_ = system.SafeRemove(filepath.Join(envDir, phase.BootstrapStateSentinelFile))
	outputPath := filepath.Join(envDir, "terraform.tfvars")
	return renderAndWrite(
		func() (string, error) { return templates.RenderTerraformVars(&data) },
		outputPath, 0o600, "terraform.tfvars",
	)
}
