package setup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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
		HAEnabled:            proxmox.HAEnabled,
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

	envDir := filepath.Join(opts.ProjectRoot, "infrastructure", "terraform", "environments", phase.GetTerraformEnv(cfg))
	// A stale postinstall sentinel would override the regenerated
	// bootstrap_enabled=true and silently skip the bootstrap VM. Deploy is the
	// only caller that should resurrect bootstrap, so the removal lives here,
	// not in WriteTerraformVars — node-lifecycle re-renders must preserve it.
	_ = system.SafeRemove(filepath.Join(envDir, phase.BootstrapStateSentinelFile))
	return WriteTerraformVars(cfg, envDir)
}

// WriteTerraformVars renders terraform.tfvars from cfg into envDir. Unlike
// GenerateTerraformVars it does NOT touch the bootstrap sentinel, so a
// post-install re-render (node add/remove/resize) cannot resurrect the
// bootstrap VM by flipping bootstrap_enabled back to true. envDir is the
// concrete environment directory (…/environments/<env>).
func WriteTerraformVars(cfg *config.Config, envDir string) error {
	if cfg.Provider.Proxmox == nil {
		return &errtypes.ConfigError{Msg: "proxmox provider configuration required"}
	}
	data := buildTerraformVarsData(cfg)
	outputPath := filepath.Join(envDir, "terraform.tfvars")
	return renderAndWrite(
		func() (string, error) { return templates.RenderTerraformVars(&data) },
		outputPath, 0o600, "terraform.tfvars",
	)
}

// TerraformVarsSizing is the per-role cpu/memory pair last rendered into
// terraform.tfvars.
type TerraformVarsSizing struct {
	MasterCPU      int
	MasterMemoryMB int
	WorkerCPU      int
	WorkerMemoryMB int
}

// tfvarsIntAssignment matches okdctl's own generated "key = 1234" lines. It is
// not a general HCL parser — WriteTerraformVars is the only writer of this
// file's scalar fields, so a purpose-built reader for that exact shape is
// enough; hand-edited tfvars using interpolation or expressions for these
// keys will not match and ReadTerraformVarsSizing reports it as missing.
var tfvarsIntAssignment = regexp.MustCompile(`(?m)^(\w+)\s*=\s*(-?\d+)\s*$`)

// ReadTerraformVarsSizing parses the four scalar sizing fields WriteTerraformVars
// renders out of envDir's terraform.tfvars, so `okdctl node list` can detect
// drift between the live config and the sizing last materialized to disk.
// found=false (with a nil error) means terraform.tfvars has not been rendered
// yet — a fresh workspace has nothing to drift from.
func ReadTerraformVarsSizing(envDir string) (sizing TerraformVarsSizing, found bool, err error) {
	path := filepath.Join(envDir, "terraform.tfvars")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return TerraformVarsSizing{}, false, nil
	}
	if err != nil {
		return TerraformVarsSizing{}, false, fmt.Errorf("read terraform.tfvars: %w", err)
	}

	values := map[string]int{}
	for _, m := range tfvarsIntAssignment.FindAllStringSubmatch(string(data), -1) {
		if v, convErr := strconv.Atoi(m[2]); convErr == nil {
			values[m[1]] = v
		}
	}

	fields := map[string]*int{
		"master_cpu_cores": &sizing.MasterCPU,
		"master_memory_mb": &sizing.MasterMemoryMB,
		"worker_cpu_cores": &sizing.WorkerCPU,
		"worker_memory_mb": &sizing.WorkerMemoryMB,
	}
	var missing []string
	for key, dst := range fields {
		v, ok := values[key]
		if !ok {
			missing = append(missing, key)
			continue
		}
		*dst = v
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return TerraformVarsSizing{}, false, fmt.Errorf("parse terraform.tfvars: missing sizing key(s) %s", strings.Join(missing, ", "))
	}
	return sizing, true, nil
}
