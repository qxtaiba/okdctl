package steps

import (
	"fmt"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// ResourcesStepState pairs the resources step with the Config it edits, so
// callers can inspect values after the wizard completes.
type ResourcesStepState struct {
	Step *wizard.DataDrivenStep
	Cfg  *config.Config
}

// IsWizardStepState marks ResourcesStepState as a valid wizard.StepState.
func (s *ResourcesStepState) IsWizardStepState() {}

// nodeResourceRole bundles what differs between the control-plane and
// workers field blocks below (key prefix, defaults/help text, and which
// NodeConfig a field writes/reads); node must return a pointer into cfg's
// Topology so ConfigSet mutates the caller's Config in place.
type nodeResourceRole struct {
	prefix                string
	vcpuDefault, vcpuHelp string
	memDefault, memHelp   string
	diskHelp              string
	node                  func(cfg *config.Config) *config.NodeConfig
}

// nodeResourceFields declares the vcpus/memory/os-disk fields for one
// topology role. Bootstrap disk is intentionally not seeded by the disk
// field here: it always mirrors control-plane disk, resolved once at point
// of use by config.Effective/NormalizeTopology — writing it here too would
// materialize a value that later desyncs when control-plane disk changes
// outside the wizard (e.g. 'okdctl node resize') without this field running again.
func nodeResourceFields(role *nodeResourceRole) []wizard.FieldDefinition {
	return []wizard.FieldDefinition{
		{
			Key:       role.prefix + "_vcpus",
			Label:     "vcpus",
			Default:   role.vcpuDefault,
			Help:      role.vcpuHelp,
			Required:  true,
			Validate:  config.ValidateCPU,
			ConfigSet: wizard.SetInt(func(c *config.Config, v int) { role.node(c).CPU = v }),
			ConfigGet: wizard.GetInt(func(c *config.Config) int { return role.node(c).CPU }),
		},
		{
			Key:       role.prefix + "_memory",
			Label:     "memory (mb)",
			Default:   role.memDefault,
			Help:      role.memHelp,
			Required:  true,
			Validate:  config.ValidateMemory,
			ConfigSet: wizard.SetInt(func(c *config.Config, v int) { role.node(c).MemoryMB = v }),
			ConfigGet: wizard.GetInt(func(c *config.Config) int { return role.node(c).MemoryMB }),
		},
		{
			Key:       role.prefix + "_disk",
			Label:     "os disk (gb)",
			Default:   "50",
			Help:      role.diskHelp,
			Required:  true,
			Validate:  config.ValidateOSDisk,
			ConfigSet: wizard.SetInt(func(c *config.Config, v int) { role.node(c).DiskGB = v }),
			ConfigGet: wizard.GetInt(func(c *config.Config) int { return role.node(c).DiskGB }),
		},
	}
}

// ResourcesStepDefinition declares the node-resources step fields.
var ResourcesStepDefinition = wizard.StepDefinition{
	ID:           wizard.StepIDResources,
	Title:        "node resources",
	DisplayTitle: "configure node resources",
	Description:  "configure cpu, memory, and storage for nodes",
	Sections: []wizard.SectionDefinition{
		{
			Title: roleLabelControlPlane,
			Fields: nodeResourceFields(&nodeResourceRole{
				prefix:      "cp",
				vcpuDefault: "4", vcpuHelp: "okd minimum: 4 vcpus",
				memDefault: "12288", memHelp: "okd minimum: 8192 mb (8 gb)",
				diskHelp: "boot disk for control plane nodes (okd minimum: 50 gb)",
				node:     func(cfg *config.Config) *config.NodeConfig { return &cfg.Topology.ControlPlane },
			}),
		},
		{
			Title: roleLabelWorkers,
			Fields: nodeResourceFields(&nodeResourceRole{
				prefix:      "worker",
				vcpuDefault: "8", vcpuHelp: "recommended: 4-16 vcpus",
				memDefault: "20480", memHelp: "recommended: 8192-65536 mb",
				diskHelp: "boot disk for worker nodes (okd minimum: 50 gb)",
				node:     func(cfg *config.Config) *config.NodeConfig { return &cfg.Topology.Workers },
			}),
		},
		{
			Title: fieldDataStorage,
			Fields: []wizard.FieldDefinition{
				{
					Key:       "worker_data_disk",
					Label:     "worker data disk (gb)",
					Default:   "500",
					Help:      "data disk per worker for ceph/storage — set to 0 to disable",
					Required:  true,
					Validate:  config.ValidateDataDisk,
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.Disks.WorkerDataSizeGB = v }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.Disks.WorkerDataSizeGB }),
				},
				{
					Key:       "cp_data_disk",
					Label:     "control plane data disk (gb)",
					Default:   "0",
					Help:      "data disk per control plane node for ceph/storage — set to 0 to disable",
					Required:  true,
					Validate:  config.ValidateDataDisk,
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.Disks.ControlPlaneDataSizeGB = v }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.Disks.ControlPlaneDataSizeGB }),
				},
			},
		},
	},
}

// NewResourcesStep returns the resources wizard step and its state.
func NewResourcesStep() (*wizard.DataDrivenStep, *ResourcesStepState) {
	step := wizard.NewDataDrivenStep(&ResourcesStepDefinition)

	state := &ResourcesStepState{
		Step: step,
	}

	step.WithExtraContentFunc(func(s *wizard.DataDrivenStep, width int) string {
		return renderResourceSummary(s, state, width)
	})

	return step, state
}

var resourceSummaryStyles = struct {
	wrapper lipgloss.Style
	title   lipgloss.Style
	value   lipgloss.Style
	sep     string
}{
	wrapper: lipgloss.NewStyle().Padding(1, 2),
	title:   lipgloss.NewStyle().Foreground(tui.ColorTextDim).Bold(true),
	value:   lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true),
	sep:     lipgloss.NewStyle().Foreground(tui.ColorBorder).Render("  ·  "),
}

func renderResourceSummary(step *wizard.DataDrivenStep, state *ResourcesStepState, width int) string {
	cpCount := 3
	workerCount := 3
	if state.Cfg != nil {
		cpCount = state.Cfg.Topology.ControlPlane.Count
		workerCount = state.Cfg.Topology.Workers.Count
	}

	cpCPU := step.ValueInt("cp_vcpus", 4)
	cpMem := step.ValueInt("cp_memory", 12288)
	cpDisk := step.ValueInt("cp_disk", 50)
	workerCPU := step.ValueInt("worker_vcpus", 8)
	workerMem := step.ValueInt("worker_memory", 20480)
	workerDisk := step.ValueInt("worker_disk", 50)
	workerDataDisk := step.ValueInt("worker_data_disk", 500)
	cpDataDisk := step.ValueInt("cp_data_disk", 0)

	totalCPU := (cpCPU * cpCount) + (workerCPU * workerCount)
	totalMem := (cpMem * cpCount) + (workerMem * workerCount)
	totalOSDisk := (cpDisk * cpCount) + (workerDisk * workerCount)
	totalDataDisk := (workerDataDisk * workerCount) + (cpDataDisk * cpCount)

	boxContentWidth := max(width-8, 30)

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.ColorBorder).
		Padding(0, 1).
		Width(boxContentWidth)

	sep := resourceSummaryStyles.sep

	var storageStr string
	if totalDataDisk >= 1000 {
		storageStr = fmt.Sprintf("%.1f tb storage", float64(totalDataDisk)/1000)
	} else {
		storageStr = fmt.Sprintf("%d gb storage", totalDataDisk)
	}

	summary := resourceSummaryStyles.title.Render("total resources required") + "\n\n" +
		resourceSummaryStyles.value.Render(fmt.Sprintf("%d vcpus", totalCPU)) + sep +
		resourceSummaryStyles.value.Render(fmt.Sprintf("%d gb ram", totalMem/1024)) + sep +
		resourceSummaryStyles.value.Render(fmt.Sprintf("%d gb os", totalOSDisk)) + sep +
		resourceSummaryStyles.value.Render(storageStr)

	return resourceSummaryStyles.wrapper.Render(boxStyle.Render(summary))
}
