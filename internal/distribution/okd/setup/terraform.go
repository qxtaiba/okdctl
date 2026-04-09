package setup

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/paths"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/templates"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

func buildISOStrings(isoStorage string, role string, count int) []string {
	isos := make([]string, count)
	for i := range count {
		isos[i] = fmt.Sprintf(`"%s:iso/%s%d.iso"`, isoStorage, role, i)
	}
	return isos
}

func buildNodeNames(clusterName, role string, count int) []string {
	names := make([]string, count)
	for i := range count {
		names[i] = fmt.Sprintf(`"%s-%s%d"`, clusterName, role, i)
	}
	return names
}

func getDiskSizes(cfg *config.Config) (cpDisk, workerDisk, dataDisk int) {
	cpDisk = cfg.Topology.ControlPlane.Disk
	if cpDisk == 0 {
		cpDisk = 50
	}
	workerDisk = cfg.Topology.Workers.Disk
	if workerDisk == 0 {
		workerDisk = cpDisk
	}
	dataDisk = cfg.Disks.DataSizeGB
	if dataDisk == 0 {
		dataDisk = 500
	}
	return cpDisk, workerDisk, dataDisk
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
	cpDisk, workerDisk, dataDisk := getDiskSizes(cfg)
	bootstrapCPU, bootstrapMem := getBootstrapResources(cfg)

	masterISOs := buildISOStrings(proxmox.ISOStorage, "master", cfg.Topology.ControlPlane.Count)
	workerISOs := buildISOStrings(proxmox.ISOStorage, "worker", cfg.Topology.Workers.Count)
	masterNames := buildNodeNames(cfg.Cluster.Name, "master", cfg.Topology.ControlPlane.Count)
	workerNames := buildNodeNames(cfg.Cluster.Name, "worker", cfg.Topology.Workers.Count)

	return templates.TerraformVarsData{
		ClusterName:        cfg.Cluster.Name,
		TargetNode:         proxmox.Node,
		Bridge:             proxmox.Bridge,
		OSStorage:          proxmox.Storage,
		DataStorage:        proxmox.DataStorage,
		FCOSISOStorage:     proxmox.ISOStorage,
		MasterISOsString:   strings.Join(masterISOs, ", "),
		WorkerISOsString:   strings.Join(workerISOs, ", "),
		VMIDBase:           cfg.Topology.VMIDBase,
		MasterCount:        cfg.Topology.ControlPlane.Count,
		WorkerCount:        cfg.Topology.Workers.Count,
		OSDiskSizeGB:       cpDisk,
		MasterOSDiskSizeGB: cpDisk,
		WorkerOSDiskSizeGB: workerDisk,
		DataDiskSizeGB:     dataDisk,
		BootstrapCPUCores:  bootstrapCPU,
		BootstrapMemoryMB:  bootstrapMem,
		MasterCPUCores:     cfg.Topology.ControlPlane.CPU,
		MasterMemoryMB:     cfg.Topology.ControlPlane.Memory,
		WorkerCPUCores:     cfg.Topology.Workers.CPU,
		WorkerMemoryMB:     cfg.Topology.Workers.Memory,
		MasterNames:        strings.Join(masterNames, ", "),
		WorkerNames:        strings.Join(workerNames, ", "),
	}
}


func (p *Phase) GenerateTerraformVars(cfg *config.Config, opts Options) error {
	if cfg.Provider.Proxmox == nil {
		return fmt.Errorf("proxmox provider configuration required")
	}

	data := buildTerraformVarsData(cfg)
	content, err := templates.RenderTerraformVars(data)
	if err != nil {
		return fmt.Errorf("failed to render terraform.tfvars template: %w", err)
	}

	outputPath := filepath.Join(opts.ProjectRoot, "infrastructure", "terraform", "environments", paths.GetTerraformEnv(cfg), "terraform.tfvars")

	if err := system.AtomicWriteString(outputPath, content, 0644); err != nil {
		return fmt.Errorf("failed to write terraform.tfvars: %w", err)
	}
	return nil
}
