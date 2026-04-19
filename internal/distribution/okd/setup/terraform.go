package setup

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/templates"
	"github.com/qxtaiba/okdctl/internal/errtypes"
)

// DefaultProxmoxCPUType is the Proxmox qemu cpu type used when the operator
// has not set proxmox.cpuType. "host" passes every CPU flag through so nested
// virt and vector extensions (AVX2, AES-NI) required by OKD nodes work.
const DefaultProxmoxCPUType = "host"

func buildISOStrings(isoStorage string, role phase.NodeRole, count int) []string {
	isos := make([]string, count)
	for i := range count {
		isos[i] = fmt.Sprintf(`"%s:iso/%s%d.iso"`, isoStorage, role, i)
	}
	return isos
}

func buildNodeNames(clusterName string, role phase.NodeRole, count int) []string {
	names := make([]string, count)
	for i := range count {
		names[i] = fmt.Sprintf(`"%s-%s%d"`, clusterName, role, i)
	}
	return names
}

func getDiskSizes(cfg *config.Config) (cpDisk, workerDisk, workerDataDisk, masterDataDisk int) {
	cpDisk = cfg.Topology.ControlPlane.Disk
	if cpDisk == 0 {
		cpDisk = 50
	}
	workerDisk = cfg.Topology.Workers.Disk
	if workerDisk == 0 {
		workerDisk = cpDisk
	}
	workerDataDisk = cfg.Disks.WorkerDataSizeGB
	masterDataDisk = cfg.Disks.MasterDataSizeGB
	return cpDisk, workerDisk, workerDataDisk, masterDataDisk
}

func getBootstrapResources(cfg *config.Config) (cpu, mem int) {
	cpu = cfg.Topology.Bootstrap.CPU
	mem = cfg.Topology.Bootstrap.Memory
	if cpu == 0 {
		cpu = cfg.Topology.ControlPlane.CPU
	}
	if mem == 0 {
		mem = cfg.Topology.ControlPlane.Memory
	}
	return cpu, mem
}

func buildTerraformVarsData(cfg *config.Config) templates.TerraformVarsData {
	proxmox := cfg.Provider.Proxmox
	cpDisk, workerDisk, workerDataDisk, masterDataDisk := getDiskSizes(cfg)
	bootstrapCPU, bootstrapMem := getBootstrapResources(cfg)

	masterISOs := buildISOStrings(proxmox.ISOStorage, phase.RoleMaster, cfg.Topology.ControlPlane.Count)
	workerISOs := buildISOStrings(proxmox.ISOStorage, phase.RoleWorker, cfg.Topology.Workers.Count)
	masterNames := buildNodeNames(cfg.Cluster.Name, phase.RoleMaster, cfg.Topology.ControlPlane.Count)
	workerNames := buildNodeNames(cfg.Cluster.Name, phase.RoleWorker, cfg.Topology.Workers.Count)

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
		OSDiskSizeGB:         cpDisk,
		MasterOSDiskSizeGB:   cpDisk,
		WorkerOSDiskSizeGB:   workerDisk,
		WorkerDataDiskSizeGB: workerDataDisk,
		MasterDataDiskSizeGB: masterDataDisk,
		BootstrapCPUCores:    bootstrapCPU,
		BootstrapMemoryMB:    bootstrapMem,
		MasterCPUCores:       cfg.Topology.ControlPlane.CPU,
		MasterMemoryMB:       cfg.Topology.ControlPlane.Memory,
		WorkerCPUCores:       cfg.Topology.Workers.CPU,
		WorkerMemoryMB:       cfg.Topology.Workers.Memory,
		MasterNames:          strings.Join(masterNames, ", "),
		WorkerNames:          strings.Join(workerNames, ", "),
		CPUType:              cpuType,
		NUMAEnabled:          proxmox.NUMAEnabled,
		AdditionalNetworks:   formatAdditionalNetworks(proxmox.AdditionalNetworks),
		MasterTargetNodes:    formatStringList(proxmox.MasterNodes),
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
func (p *Phase) GenerateTerraformVars(cfg *config.Config, opts *Options) error {
	if cfg.Provider.Proxmox == nil {
		return &errtypes.ConfigError{Msg: "proxmox provider configuration required"}
	}

	data := buildTerraformVarsData(cfg)
	outputPath := filepath.Join(opts.ProjectRoot, "infrastructure", "terraform", "environments", phase.GetTerraformEnv(cfg), "terraform.tfvars")
	return renderAndWrite(
		func() (string, error) { return templates.RenderTerraformVars(&data) },
		outputPath, 0o600, "terraform.tfvars",
	)
}
