package steps

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui/wizard"
)

type ResourcesStepState struct {
	Step *wizard.DataDrivenStep
	Cfg  *config.Config
}

var ResourcesStepDefinition = wizard.StepDefinition{
	ID:           "resources",
	Title:        "node resources",
	DisplayTitle: "configure node resources",
	Description:  "configure cpu, memory, and storage for nodes",
	Sections: []wizard.SectionDefinition{
		{
			Title: "control plane",
			Fields: []wizard.FieldDefinition{
				{
					Key:       "cp_vcpus",
					Label:     "vcpus",
					Default:   "4",
					Help:      "okd minimum: 4 vcpus",
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
					Required:  true,
					Validate:  config.ValidateMemory,
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.Topology.ControlPlane.Memory = v }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.Topology.ControlPlane.Memory }),
				},
				{
					Key:      "cp_disk",
					Label:    "os disk (gb)",
					Default:  "50",
					Help:     "boot disk for control plane nodes (okd minimum: 50 gb)",
					Required: true,
					Validate: config.ValidateOSDisk,
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) {
						c.Topology.ControlPlane.Disk = v
						c.Topology.Bootstrap.Disk = v
					}),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.Topology.ControlPlane.Disk }),
				},
			},
		},
		{
			Title: "workers",
			Fields: []wizard.FieldDefinition{
				{
					Key:       "worker_vcpus",
					Label:     "vcpus",
					Default:   "8",
					Help:      "recommended: 4-16 vcpus",
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
					Required:  true,
					Validate:  config.ValidateMemory,
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.Topology.Workers.Memory = v }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.Topology.Workers.Memory }),
				},
				{
					Key:       "worker_disk",
					Label:     "os disk (gb)",
					Default:   "50",
					Help:      "boot disk for worker nodes (okd minimum: 50 gb)",
					Required:  true,
					Validate:  config.ValidateOSDisk,
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.Topology.Workers.Disk = v }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.Topology.Workers.Disk }),
				},
			},
		},
		{
			Title: "storage",
			Fields: []wizard.FieldDefinition{
				{
					Key:       "ceph_disk",
					Label:     "ceph data disk (gb)",
					Default:   "500",
					Help:      "dedicated ceph osd disk per worker (50-5000 gb)",
					Required:  true,
					Validate:  config.ValidateDataDisk,
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.Disks.DataSizeGB = v }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.Disks.DataSizeGB }),
				},
			},
		},
	},
}

func NewResourcesStep() (*wizard.DataDrivenStep, *ResourcesStepState) {
	step := wizard.NewDataDrivenStep(ResourcesStepDefinition)

	state := &ResourcesStepState{
		Step: step,
	}

	step.WithExtraContentFunc(func(s *wizard.DataDrivenStep, width int) string {
		return renderResourceSummary(s, state, width)
	})

	return step, state
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
	cephDisk := step.ValueInt("ceph_disk", 500)

	totalCPU := (cpCPU * cpCount) + (workerCPU * workerCount)
	totalMem := (cpMem * cpCount) + (workerMem * workerCount)
	totalOSDisk := (cpDisk * cpCount) + (workerDisk * workerCount)
	totalCephDisk := cephDisk * workerCount

	wrapperStyle := lipgloss.NewStyle().Padding(1, 2)

	boxContentWidth := width - 8
	if boxContentWidth < 30 {
		boxContentWidth = 30
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.ColorSlate600).
		Padding(0, 1).
		Width(boxContentWidth)

	titleStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate400).Bold(true)
	valueStyle := lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true)
	sepStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate600)
	sep := sepStyle.Render("  ·  ")

	var storageStr string
	if totalCephDisk >= 1000 {
		storageStr = fmt.Sprintf("%.1f tb storage", float64(totalCephDisk)/1000)
	} else {
		storageStr = fmt.Sprintf("%d gb storage", totalCephDisk)
	}

	summary := titleStyle.Render("total resources required") + "\n\n" +
		valueStyle.Render(fmt.Sprintf("%d vcpus", totalCPU)) + sep +
		valueStyle.Render(fmt.Sprintf("%d gb ram", totalMem/1024)) + sep +
		valueStyle.Render(fmt.Sprintf("%d gb os", totalOSDisk)) + sep +
		valueStyle.Render(storageStr)

	return wrapperStyle.Render(boxStyle.Render(summary))
}
