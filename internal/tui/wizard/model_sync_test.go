package wizard

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
)

type syncStep struct {
	id         StepID
	applyCalls int
}

func (s *syncStep) ID() StepID                           { return s.id }
func (s *syncStep) Title() string                        { return string(s.id) }
func (s *syncStep) Init() tea.Cmd                        { return nil }
func (s *syncStep) Update(tea.Msg) (WizardStep, tea.Cmd) { return s, nil }
func (s *syncStep) View(int, int) string                 { return "" }
func (s *syncStep) Apply(*config.Config) error           { s.applyCalls++; return nil }

func TestConfigSyncAppliesCurrentStep(t *testing.T) {
	step := &syncStep{id: StepIDBasics}
	model := NewFlowModel([]WizardStep{step}, config.DefaultConfig(), DefaultChrome())
	model.Update(ConfigSyncMsg{StepID: StepIDBasics})
	if step.applyCalls != 1 {
		t.Fatalf("apply calls=%d", step.applyCalls)
	}
}

func TestConfigSyncSkipsFieldsMidEditAndNeverPaintsTheStatusRow(t *testing.T) {
	step := NewDataDrivenStep(&StepDefinition{
		ID:    StepIDResources,
		Title: "resources",
		Sections: []SectionDefinition{{Fields: []FieldDefinition{
			{Key: "cpu", Label: "cpu", Default: "4", Required: true, ConfigSet: SetInt(func(c *config.Config, v int) { c.ControlPlane.CPU = v })},
			{Key: "mem", Label: "mem", Default: "16384", Required: true, ConfigSet: SetInt(func(c *config.Config, v int) { c.ControlPlane.MemoryMiB = v })},
		}}},
	})
	cfg := config.DefaultConfig()
	m := NewModel([]WizardStep{step}, cfg)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	step.SetValue("mem", "32768")

	m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m.Update(ConfigSyncMsg{StepID: StepIDResources})

	if m.err != nil {
		t.Fatalf("status row = %v after a sync with a field mid-edit, want nothing painted", m.err)
	}
	if cfg.ControlPlane.CPU != 4 || cfg.ControlPlane.MemoryMiB != 32768 {
		t.Fatalf("cfg cpu=%d mem=%d, want the blank cpu skipped and the later field still synced", cfg.ControlPlane.CPU, cfg.ControlPlane.MemoryMiB)
	}
}
