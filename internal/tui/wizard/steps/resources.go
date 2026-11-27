package steps

import (
	"fmt"
	"maps"
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

// ResourcesStepState pairs the resources step with the Config its footer
// totals read.
type ResourcesStepState struct {
	Step     *wizard.DataDrivenStep
	Cfg      *config.Config
	Capacity *WizardCapacitySnapshot
}

// IsWizardStepState marks ResourcesStepState as a wizard.StepState.
func (s *ResourcesStepState) IsWizardStepState() {}

// ResourcesStepDefinition declares the control-plane and worker sizing fields.
var ResourcesStepDefinition = wizard.StepDefinition{
	ID:           wizard.StepIDResources,
	Title:        "node resources",
	DisplayTitle: "configure node resources",
	Description:  "size the three control-plane nodes and the worker pool",
	Sections: []wizard.SectionDefinition{
		{
			Title:  roleLabelControlPlane,
			Note:   "always three nodes",
			Fields: sizingFields(config.RoleMaster, [3]string{fieldCPCPU, fieldCPMemory, fieldCPRootDisk}, pairKeyControlPlaneResources, "4", func(c *config.Config) *config.NodeDefaults { return &c.ControlPlane }),
		},
		{
			Title: roleLabelWorkers,
			Fields: slices.Concat(
				[]wizard.FieldDefinition{{
					Key:       fieldWorkerCount,
					Label:     "count",
					Default:   "2",
					Help:      "number of worker nodes — 0 runs the workloads on the control plane",
					Width:     wizard.FieldWidthNumber,
					Required:  true,
					Validate:  minInt(0, ""),
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.Workers.Count = v }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.Workers.Count }),
				}},
				sizingFields(config.RoleWorker, [3]string{fieldWorkerCPU, fieldWorkerMemory, fieldWorkerRootDisk}, pairKeyWorkerResources, "8", func(c *config.Config) *config.NodeDefaults { return &c.Workers.NodeDefaults }),
				[]wizard.FieldDefinition{{
					Key:       fieldWorkerDataDisk,
					Label:     "data disk (gib)",
					Default:   "500",
					Help:      "second disk per worker for ceph or local storage — 0 for none",
					Width:     wizard.FieldWidthNumber,
					Required:  true,
					Validate:  minInt(0, "gib"),
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.Workers.Disks = setDiskSize(c.Workers.Disks, 1, v) }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return diskSize(c.Workers.Disks, 1) }),
				}},
			),
		},
	},
	Apply: func(_ *wizard.DataDrivenStep, cfg *config.Config) error {
		if _, defaults, err := placementInventory(cfg); err == nil {
			pruneStaleHostOverrides(cfg, defaults)
		}
		return nil
	},
}

// sizingFields declares a role's cpu, memory and root-disk fields, floored
// at config.FloorFor(role) and bound to the defaults block of returns.
func sizingFields(role config.Role, keys [3]string, pairKey, cpuDefault string, of func(*config.Config) *config.NodeDefaults) []wizard.FieldDefinition {
	floor := config.FloorFor(role)
	return []wizard.FieldDefinition{
		{
			Key:       keys[0],
			Label:     labelCPU,
			Default:   cpuDefault,
			Help:      fmt.Sprintf("okd minimum: %d vcpus", floor.CPU),
			Width:     wizard.FieldWidthNumber,
			Required:  true,
			PairKey:   pairKey,
			Validate:  minInt(floor.CPU, "vcpu"),
			ConfigSet: wizard.SetInt(func(c *config.Config, v int) { of(c).CPU = v }),
			ConfigGet: wizard.GetInt(func(c *config.Config) int { return of(c).CPU }),
		},
		{
			Key:       keys[1],
			Label:     labelMemory,
			Default:   "16384",
			Help:      fmt.Sprintf("okd minimum: %d mib (%d gib)", floor.MemoryMiB, floor.MemoryMiB/1024),
			Width:     wizard.FieldWidthNumber,
			Required:  true,
			PairKey:   pairKey,
			Validate:  minInt(floor.MemoryMiB, "mib"),
			ConfigSet: wizard.SetInt(func(c *config.Config, v int) { of(c).MemoryMiB = v }),
			ConfigGet: wizard.GetInt(func(c *config.Config) int { return of(c).MemoryMiB }),
		},
		{
			Key:       keys[2],
			Label:     labelRootDisk,
			Default:   "120",
			Help:      fmt.Sprintf("okd minimum: %d gib", floor.RootGiB),
			Width:     wizard.FieldWidthNumber,
			Required:  true,
			PairKey:   pairKey,
			Validate:  minInt(floor.RootGiB, "gib"),
			ConfigSet: wizard.SetInt(func(c *config.Config, v int) { d := of(c); d.Disks = setDiskSize(d.Disks, 0, v) }),
			ConfigGet: wizard.GetInt(func(c *config.Config) int { return diskSize(of(c).Disks, 0) }),
		},
	}
}

// setDiskSize sizes disks[i], growing the list; a zero size on a non-root
// index drops that disk and the ones after it.
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

// NewResourcesStep returns the resources step and its state.
func NewResourcesStep(capacity *WizardCapacitySnapshot) (*wizard.DataDrivenStep, *ResourcesStepState) {
	step := wizard.NewDataDrivenStep(&ResourcesStepDefinition)
	state := &ResourcesStepState{Step: step, Capacity: capacity}
	step.WithPinnedFooterFunc(func(s *wizard.DataDrivenStep, width int) string {
		return renderResourceFooter(s, state, width)
	})
	return step, state
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
	overlay.Nodes = maps.Clone(overlay.Nodes)
	if !step.IsComplete() || step.Apply(&overlay) != nil {
		return tui.Truncate("resource totals pending · enter valid values", width)
	}
	inv, err := model.Build(&overlay, model.PreviewInfraID)
	if err != nil {
		return tui.Truncate("resource totals pending · enter valid values", width)
	}
	total := inventoryDemand(inv.Nodes)
	label := fmt.Sprintf("%d vcpu · %d gb ram · %d gb disk", total.cpu, total.memoryMiB/1024, total.diskGiB)
	capacity := state.Capacity.OnlineTotals()
	if capacity.overCPU(&total) || capacity.overMemory(&total) {
		label = tui.Truncate(label+fmt.Sprintf(" · exceeds online capacity (%dc/%dg)", capacity.CPUs, capacity.MemoryGB), width)
		return lipgloss.NewStyle().Foreground(tui.ColorWarning()).Bold(true).Render(label)
	}
	return lipgloss.NewStyle().Foreground(tui.ColorPrimary()).Bold(true).Render(strings.TrimSpace(tui.Truncate(label, width)))
}
