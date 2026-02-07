package steps

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui/wizard"
)

// ResourcesStepState holds node counts from the basics step for resource summary rendering.
type ResourcesStepState struct {
	Step        *wizard.DataDrivenStep
	CPCount     int
	WorkerCount int
}

// ResourcesStepDefinition defines the resources configuration step declaratively.
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
			},
		},
		{
			Title: "storage",
			Fields: []wizard.FieldDefinition{
				{
					Key:      "os_disk",
					Label:    "os disk (gb)",
					Default:  "50",
					Help:     "boot disk for all nodes (okd minimum: 50 gb)",
					Required: true,
					Validate: config.ValidateOSDisk,
					ConfigSet: func(cfg *config.Config, value string) error {
						v, err := parseIntValue(value)
						if err != nil {
							return err
						}
						cfg.Disks.OSSizeGB = v
						cfg.Topology.ControlPlane.Disk = v
						cfg.Topology.Workers.Disk = v
						cfg.Topology.Bootstrap.Disk = v
						return nil
					},
					ConfigGet: func(cfg *config.Config) string {
						v := cfg.Disks.OSSizeGB
						if v == 0 {
							v = cfg.Topology.ControlPlane.Disk
						}
						if v == 0 {
							return ""
						}
						return fmt.Sprintf("%d", v)
					},
				},
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

// parseIntValue parses a string to int for ConfigSet.
func parseIntValue(value string) (int, error) {
	var v int
	_, err := fmt.Sscanf(value, "%d", &v)
	return v, err
}

// NewResourcesStep creates a new resources configuration step.
func NewResourcesStep() (*wizard.DataDrivenStep, *ResourcesStepState) {
	step := wizard.NewDataDrivenStep(ResourcesStepDefinition)

	state := &ResourcesStepState{
		Step:        step,
		CPCount:     3,
		WorkerCount: 3,
	}

	step.WithExtraContentFunc(func(s *wizard.DataDrivenStep, width int) string {
		return renderResourceSummary(s, state, width)
	})

	return step, state
}

func renderResourceSummary(step *wizard.DataDrivenStep, state *ResourcesStepState, width int) string {
	cpCPU := step.ValueInt("cp_vcpus", 4)
	cpMem := step.ValueInt("cp_memory", 12288)
	workerCPU := step.ValueInt("worker_vcpus", 8)
	workerMem := step.ValueInt("worker_memory", 20480)
	osDisk := step.ValueInt("os_disk", 50)
	cephDisk := step.ValueInt("ceph_disk", 500)

	totalCPU := (cpCPU * state.CPCount) + (workerCPU * state.WorkerCount)
	totalMem := (cpMem * state.CPCount) + (workerMem * state.WorkerCount)
	totalNodes := state.CPCount + state.WorkerCount
	totalOSDisk := osDisk * totalNodes
	totalCephDisk := cephDisk * state.WorkerCount

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

