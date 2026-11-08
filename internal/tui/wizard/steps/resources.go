package steps

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/model"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

const (
	pairKeyControlPlaneResources = "cp_resources"
	pairKeyWorkerResources       = "worker_resources"
)

// ResourcesStepState pairs the resources step with the Config it edits so the
// footer totals read the same config the review does.
type ResourcesStepState struct {
	Step     *wizard.DataDrivenStep
	Cfg      *config.Config
	Capacity *WizardCapacitySnapshot
}

// IsWizardStepState marks ResourcesStepState as a valid wizard.StepState.
func (s *ResourcesStepState) IsWizardStepState() {}

// ResourcesStepDefinition declares the control-plane and worker sizing fields.
var ResourcesStepDefinition = wizard.StepDefinition{
	ID:           wizard.StepIDResources,
	Title:        "node resources",
	DisplayTitle: "configure node resources",
	Description:  "size the three control-plane nodes and the worker pool",
	Sections: []wizard.SectionDefinition{
		{
			Title: roleLabelControlPlane,
			Note:  "always three nodes",
			Fields: []wizard.FieldDefinition{
				{
					Key:       fieldCPCPU,
					Label:     labelCPU,
					Default:   "4",
					Help:      "okd minimum: 4 vcpus",
					Width:     wizard.FieldWidthNumber,
					Required:  true,
					PairKey:   pairKeyControlPlaneResources,
					Validate:  minInt(4, "vcpu"),
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.ControlPlane.CPU = v }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.ControlPlane.CPU }),
				},
				{
					Key:       fieldCPMemory,
					Label:     labelMemory,
					Default:   "16384",
					Help:      "okd minimum: 16384 mib (16 gib)",
					Width:     wizard.FieldWidthNumber,
					Required:  true,
					PairKey:   pairKeyControlPlaneResources,
					Validate:  minInt(16384, "mib"),
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.ControlPlane.MemoryMiB = v }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.ControlPlane.MemoryMiB }),
				},
				{
					Key:       fieldCPRootDisk,
					Label:     labelRootDisk,
					Default:   "120",
					Help:      "okd minimum: 100 gib",
					Width:     wizard.FieldWidthNumber,
					Required:  true,
					PairKey:   pairKeyControlPlaneResources,
					Validate:  minInt(100, "gib"),
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.ControlPlane.Disks = setDiskSize(c.ControlPlane.Disks, 0, v) }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return diskSize(c.ControlPlane.Disks, 0) }),
				},
			},
		},
		{
			Title: roleLabelWorkers,
			Fields: []wizard.FieldDefinition{
				{
					Key:       fieldWorkerCount,
					Label:     "count",
					Default:   "2",
					Help:      "number of worker nodes — 0 runs the workloads on the control plane",
					Width:     wizard.FieldWidthNumber,
					Required:  true,
					Validate:  minInt(0, ""),
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.Workers.Count = v }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.Workers.Count }),
				},
				{
					Key:       fieldWorkerCPU,
					Label:     labelCPU,
					Default:   "8",
					Help:      "okd minimum: 2 vcpus",
					Width:     wizard.FieldWidthNumber,
					Required:  true,
					PairKey:   pairKeyWorkerResources,
					Validate:  minInt(2, "vcpu"),
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.Workers.CPU = v }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.Workers.CPU }),
				},
				{
					Key:       fieldWorkerMemory,
					Label:     labelMemory,
					Default:   "16384",
					Help:      "okd minimum: 8192 mib (8 gib)",
					Width:     wizard.FieldWidthNumber,
					Required:  true,
					PairKey:   pairKeyWorkerResources,
					Validate:  minInt(8192, "mib"),
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.Workers.MemoryMiB = v }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.Workers.MemoryMiB }),
				},
				{
					Key:       fieldWorkerRootDisk,
					Label:     labelRootDisk,
					Default:   "120",
					Help:      "okd minimum: 100 gib",
					Width:     wizard.FieldWidthNumber,
					Required:  true,
					PairKey:   pairKeyWorkerResources,
					Validate:  minInt(100, "gib"),
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.Workers.Disks = setDiskSize(c.Workers.Disks, 0, v) }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return diskSize(c.Workers.Disks, 0) }),
				},
				{
					Key:       fieldWorkerDataDisk,
					Label:     "data disk (gib)",
					Default:   "500",
					Help:      "second disk per worker for ceph or local storage — 0 for none",
					Width:     wizard.FieldWidthNumber,
					Required:  true,
					Validate:  minInt(0, "gib"),
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.Workers.Disks = setDiskSize(c.Workers.Disks, 1, v) }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return diskSize(c.Workers.Disks, 1) }),
				},
			},
		},
	},
}

// setDiskSize sizes disks[i], growing the list as needed; a zero size on a
// non-root index drops that disk and the ones after it.
func setDiskSize(disks []config.DiskSpec, i, size int) []config.DiskSpec {
	if size == 0 && i > 0 {
		if len(disks) > i {
			return disks[:i]
		}
		return disks
	}
	for len(disks) <= i {
		disks = append(disks, config.DiskSpec{})
	}
	disks[i].SizeGiB = size
	return disks
}

func diskSize(disks []config.DiskSpec, i int) int {
	if i < len(disks) {
		return disks[i].SizeGiB
	}
	return 0
}

// NewResourcesStep returns the resources wizard step and its state.
func NewResourcesStep(capacity *WizardCapacitySnapshot) (*wizard.DataDrivenStep, *ResourcesStepState) {
	step := wizard.NewDataDrivenStep(&ResourcesStepDefinition)
	state := &ResourcesStepState{Step: step, Capacity: capacity}
	step.WithPinnedFooterFunc(func(s *wizard.DataDrivenStep, width int) string {
		return renderResourceFooter(s, state, width)
	})
	return step, state
}

// inventoryTotals sums vcpu, memory and disk over every node of inv.
func inventoryTotals(inv *model.Inventory) (cpu, memoryMiB, diskGiB int) {
	for i := range inv.Nodes {
		n := &inv.Nodes[i]
		cpu += n.CPU
		memoryMiB += n.MemoryMiB
		for _, d := range n.Disks {
			diskGiB += d.SizeGiB
		}
	}
	return cpu, memoryMiB, diskGiB
}

// renderResourceFooter totals the as-typed sizing through model.Build on a
// copy of the session config, so the footer and the review share one sum.
func renderResourceFooter(step *wizard.DataDrivenStep, state *ResourcesStepState, width int) string {
	if state.Cfg == nil {
		return ""
	}
	overlay := *state.Cfg
	overlay.ControlPlane.Disks = slices.Clone(overlay.ControlPlane.Disks)
	overlay.Workers.Disks = slices.Clone(overlay.Workers.Disks)
	if err := step.Apply(&overlay); err != nil {
		return tui.Truncate("resource totals pending · enter valid values", width)
	}
	inv, err := model.Build(&overlay, "")
	if err != nil {
		return tui.Truncate("resource totals pending · enter valid values", width)
	}
	cpu, memoryMiB, diskGiB := inventoryTotals(&inv)
	label := fmt.Sprintf("%d vcpu · %d gb ram · %d gb disk", cpu, memoryMiB/1024, diskGiB)
	capacity := state.Capacity.OnlineTotals()
	over := (capacity.CPUsKnown && cpu > capacity.CPUs) ||
		(capacity.MemoryKnown && memoryMiB > capacity.MemoryGB*1024)
	if over {
		label = tui.Truncate(label+fmt.Sprintf(" · exceeds online capacity (%dc/%dg)", capacity.CPUs, capacity.MemoryGB), width)
		return lipgloss.NewStyle().Foreground(tui.ColorWarning()).Bold(true).Render(label)
	}
	if state.Capacity != nil && state.Capacity.discovery != nil && (!capacity.CPUsKnown || !capacity.MemoryKnown) {
		label += " · online capacity unknown"
	}
	return lipgloss.NewStyle().Foreground(tui.ColorPrimary()).Bold(true).Render(strings.TrimSpace(tui.Truncate(label, width)))
}
