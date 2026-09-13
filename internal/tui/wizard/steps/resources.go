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

// ResourcesStepDefinition declares the node-resources step fields.
var ResourcesStepDefinition = wizard.StepDefinition{
	ID:           wizard.StepIDResources,
	Title:        "node resources",
	DisplayTitle: "configure node resources",
	Description:  "configure cpu, memory, and storage for nodes",
	Sections: []wizard.SectionDefinition{
		{
			Title: roleLabelControlPlane,
			Fields: []wizard.FieldDefinition{
				{
					Key:       "cp_vcpus",
					Label:     "vcpus",
					Default:   "4",
					Help:      "okd minimum: 4 vcpus",
					Width:     wizard.FieldWidthNumber,
					Required:  true,
					Validate:  config.ValidateCPU,
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.Topology.ControlPlane.CPU = v }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.Topology.ControlPlane.CPU }),
				},
				{
					Key:       "cp_memory",
					Label:     "memory (mb)",
					Default:   "12288",
					Help:      "okd minimum: 8192 mb (8 gb)",
					Width:     wizard.FieldWidthNumber,
					Required:  true,
					Validate:  config.ValidateMemory,
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.Topology.ControlPlane.MemoryMB = v }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.Topology.ControlPlane.MemoryMB }),
				},
				{
					Key:      "cp_disk",
					Label:    "os disk (gb)",
					Default:  "50",
					Help:     "boot disk for control plane nodes (okd minimum: 50 gb)",
					Width:    wizard.FieldWidthNumber,
					Required: true,
					Validate: config.ValidateOSDisk,
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) {
						c.Topology.ControlPlane.DiskGB = v
						c.Topology.Bootstrap.DiskGB = v
					}),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.Topology.ControlPlane.DiskGB }),
				},
			},
		},
		{
			Title: roleLabelWorkers,
			Fields: []wizard.FieldDefinition{
				{
					Key:       "worker_vcpus",
					Label:     "vcpus",
					Default:   "8",
					Help:      "recommended: 4-16 vcpus",
					Width:     wizard.FieldWidthNumber,
					Required:  true,
					Validate:  config.ValidateCPU,
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.Topology.Workers.CPU = v }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.Topology.Workers.CPU }),
				},
				{
					Key:       "worker_memory",
					Label:     "memory (mb)",
					Default:   "20480",
					Help:      "recommended: 8192-65536 mb",
					Width:     wizard.FieldWidthNumber,
					Required:  true,
					Validate:  config.ValidateMemory,
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.Topology.Workers.MemoryMB = v }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.Topology.Workers.MemoryMB }),
				},
				{
					Key:       "worker_disk",
					Label:     "os disk (gb)",
					Default:   "50",
					Help:      "boot disk for worker nodes (okd minimum: 50 gb)",
					Width:     wizard.FieldWidthNumber,
					Required:  true,
					Validate:  config.ValidateOSDisk,
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.Topology.Workers.DiskGB = v }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.Topology.Workers.DiskGB }),
				},
			},
		},
		{
			Title: fieldDataStorage,
			//nolint:dupl // two int fields share the FieldDefinition shape with advanced.go's timeouts section; data, not logic
			Fields: []wizard.FieldDefinition{
				{
					Key:       "worker_data_disk",
					Label:     "worker data disk (gb)",
					Default:   "500",
					Help:      "data disk per worker for ceph/storage — set to 0 to disable",
					Width:     wizard.FieldWidthNumber,
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
					Width:     wizard.FieldWidthNumber,
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

	step.WithExtraContentFunc("total resources required", func(s *wizard.DataDrivenStep, _ int) string {
		return renderResourceSummary(s, state)
	})

	return step, state
}

var resourceSummaryStyles = struct {
	value lipgloss.Style
	sep   string
}{
	value: lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true),
	sep:   lipgloss.NewStyle().Foreground(tui.ColorSlate600).Render("  ·  "),
}

// renderResourceSummary returns the totals line for the resources step's
// info card; the card itself supplies the title and border.
func renderResourceSummary(step *wizard.DataDrivenStep, state *ResourcesStepState) string {
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

	sep := resourceSummaryStyles.sep

	var storageStr string
	if totalDataDisk >= 1000 {
		storageStr = fmt.Sprintf("%.1f tb storage", float64(totalDataDisk)/1000)
	} else {
		storageStr = fmt.Sprintf("%d gb storage", totalDataDisk)
	}

	return resourceSummaryStyles.value.Render(fmt.Sprintf("%d vcpus", totalCPU)) + sep +
		resourceSummaryStyles.value.Render(fmt.Sprintf("%d gb ram", totalMem/1024)) + sep +
		resourceSummaryStyles.value.Render(fmt.Sprintf("%d gb os", totalOSDisk)) + sep +
		resourceSummaryStyles.value.Render(storageStr)
}
