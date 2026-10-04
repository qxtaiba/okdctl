package steps

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// pairKeyControlPlaneResources and pairKeyWorkerResources group each
// role's three resource fields into one 3-up row.
const (
	pairKeyControlPlaneResources = "cp_resources"
	pairKeyWorkerResources       = "worker_resources"
)

// ResourcesStepState pairs the resources step with the Config it edits, so
// callers can inspect values after the wizard completes.
type ResourcesStepState struct {
	Step     *wizard.DataDrivenStep
	Cfg      *config.Config
	Capacity *WizardCapacitySnapshot
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
					PairKey:   pairKeyControlPlaneResources,
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
					PairKey:   pairKeyControlPlaneResources,
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
					PairKey:  "cp_resources",
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
					Help:      "okd minimum: 2 vcpus",
					Width:     wizard.FieldWidthNumber,
					Required:  true,
					PairKey:   pairKeyWorkerResources,
					Validate:  config.ValidateCPU,
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.Topology.Workers.CPU = v }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.Topology.Workers.CPU }),
				},
				{
					Key:       "worker_memory",
					Label:     "memory (mb)",
					Default:   "20480",
					Help:      "okd minimum: 8192 mb (8 gb)",
					Width:     wizard.FieldWidthNumber,
					Required:  true,
					PairKey:   pairKeyWorkerResources,
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
					PairKey:   pairKeyWorkerResources,
					Validate:  config.ValidateOSDisk,
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.Topology.Workers.DiskGB = v }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.Topology.Workers.DiskGB }),
				},
			},
		},
		{
			Title: fieldDataStorage,
			Fields: []wizard.FieldDefinition{
				{
					Key:       "worker_data_disk",
					Label:     "worker data disk (gb)",
					Default:   "500",
					Help:      "data disk per worker for ceph/storage — set to 0 to disable",
					Width:     wizard.FieldWidthNumber,
					Required:  true,
					PairKey:   "data_disks",
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
					PairKey:   "data_disks",
					Validate:  config.ValidateDataDisk,
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.Disks.ControlPlaneDataSizeGB = v }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.Disks.ControlPlaneDataSizeGB }),
				},
			},
		},
	},
}

// NewResourcesStep returns the resources wizard step and its state.
func NewResourcesStep(capacity *WizardCapacitySnapshot) (*wizard.DataDrivenStep, *ResourcesStepState) {
	step := wizard.NewDataDrivenStep(&ResourcesStepDefinition)

	state := &ResourcesStepState{
		Step:     step,
		Capacity: capacity,
	}
	step.WithPinnedFooterFunc(func(s *wizard.DataDrivenStep, width int) string {
		return renderResourceFooter(s, state, width)
	})

	return step, state
}

// resourceSummaryStyles builds the totals line's styles fresh per render —
// an eager package var would freeze its brand and separator tiers at their
// init-time polarity.
func resourceSummaryStyles() (value lipgloss.Style, sep string) {
	return lipgloss.NewStyle().Foreground(tui.ColorPrimary()).Bold(true),
		lipgloss.NewStyle().Foreground(tui.ColorSubtle()).Render("  ·  ")
}

func renderResourceFooter(step *wizard.DataDrivenStep, state *ResourcesStepState, width int) string {
	cfg := state.Cfg
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	keys := []string{"cp_vcpus", "cp_memory", "cp_disk", "worker_vcpus", "worker_memory", "worker_disk", "worker_data_disk", "cp_data_disk"}
	values := make(map[string]int, len(keys))
	for _, key := range keys {
		value, err := strconv.Atoi(step.Value(key))
		if err != nil {
			return tui.Truncate("resource totals pending · enter valid values", width)
		}
		values[key] = value
	}
	in := EffectiveResourceInputsFromConfig(cfg)
	in.ControlPlaneCPU, in.ControlPlaneMemoryMB, in.ControlPlaneDiskGB = values["cp_vcpus"], values["cp_memory"], values["cp_disk"]
	in.WorkerCPU, in.WorkerMemoryMB, in.WorkerDiskGB = values["worker_vcpus"], values["worker_memory"], values["worker_disk"]
	in.WorkerDataDiskGB, in.ControlPlaneDataDiskGB = values["worker_data_disk"], values["cp_data_disk"]
	// The bootstrap VM runs alongside the control plane during installation;
	// ComputeEffectiveResourceTotals falls back to control-plane sizing when
	// cfg carries no explicit bootstrap cpu/memory of its own.
	totals := ComputeEffectiveResourceTotals(&in)
	totalCPU, totalMemoryMB, totalDiskGB := totals.CPU, totals.MemoryMB, totals.OSDiskGB+totals.DataDiskGB
	label := fmt.Sprintf("%d vcpu · %d gb ram · %d gb disk", totalCPU, totalMemoryMB/1024, totalDiskGB)
	capacity := state.Capacity.OnlineTotals()
	over := (capacity.CPUsKnown && totalCPU > capacity.CPUs) ||
		(capacity.MemoryKnown && totalMemoryMB > capacity.MemoryGB*1024)
	if over {
		label += fmt.Sprintf(" · exceeds online capacity (%dc/%dg)", capacity.CPUs, capacity.MemoryGB)
	}
	label = tui.Truncate(label, width)
	value, _ := resourceSummaryStyles()
	if over {
		return lipgloss.NewStyle().Foreground(tui.ColorWarning()).Bold(true).Render(label)
	}
	if state.Capacity != nil && state.Capacity.discovery != nil && (!capacity.CPUsKnown || !capacity.MemoryKnown) {
		label = tui.Truncate(label+" · online capacity unknown", width)
	}
	return value.Render(strings.TrimSpace(label))
}
