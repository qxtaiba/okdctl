package wizard

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
)

func TestFrameFitsTerminal(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 40}, {40, 10}, {1, 1}} {
		m := NewModel([]WizardStep{&fakeStep{id: StepIDBasics}}, &config.Config{})
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		content := m.View().Content
		if w, h := lipgloss.Width(content), lipgloss.Height(content); w > size[0] || h > size[1] {
			t.Fatalf("terminal %v: frame %dx%d", size, w, h)
		}
	}
}

func TestFrameReservesWrappedError(t *testing.T) {
	m := NewModel([]WizardStep{&fakeStep{id: StepIDBasics}}, &config.Config{})
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.Update(ErrorSetMsg{Error: errors.New(strings.Repeat("Check configured path /Mixed/CASE. ", 12))})
	out := m.View().Content
	if lipgloss.Width(out) > 80 || lipgloss.Height(out) > 24 {
		t.Fatalf("oversized error frame: %dx%d", lipgloss.Width(out), lipgloss.Height(out))
	}
	if !strings.Contains(out, "/Mixed/CASE") {
		t.Fatal("error missing")
	}
}

func TestFocusedMixedHeightFieldSurvivesResize(t *testing.T) {
	step := NewDataDrivenStep(&StepDefinition{ID: StepIDBasics, Sections: []SectionDefinition{{Title: "settings", Fields: []FieldDefinition{
		{Key: "one", Label: "one"}, {Key: "two", Label: "two", Help: strings.Repeat("long help ", 12)}, {Key: "three", Label: "three"},
	}}}})
	step.setValue("three", "retained value")
	m := NewModel([]WizardStep{step}, &config.Config{})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	step.SetFocused(true)
	step.form.sections[0].Group.SetFocusIndex(2)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.Update(FocusChangedMsg{})
	top, bottom, ok := step.FocusBounds(68, 1000)
	if !ok || top < m.viewport.YOffset() || bottom > m.viewport.YOffset()+m.viewport.Height() {
		t.Fatalf("focused bounds [%d,%d) outside [%d,%d)", top, bottom, m.viewport.YOffset(), m.viewport.YOffset()+m.viewport.Height())
	}
	if step.Value("three") != "retained value" {
		t.Fatal("resize discarded value")
	}
}

func TestFormViewDoesNotValidate(t *testing.T) {
	probes := 0
	step := NewDataDrivenStep(&StepDefinition{ID: StepIDFiles, Sections: []SectionDefinition{{Title: "files", Fields: []FieldDefinition{{Key: "path", Label: "path", Validate: func(string) error { probes++; return nil }}}}}})
	step.setValue("path", "/fixture")
	step.SetSize(68, 20)
	step.Init()
	initial := probes
	for range 5 {
		step.View(68, 20)
	}
	if probes != initial {
		t.Fatalf("View performed %d probes", probes-initial)
	}
	step.setValue("path", "/changed")
	if err := step.Validate(); err != nil {
		t.Fatal(err)
	}
	if probes != initial+1 {
		t.Fatal("edited input did not invalidate probe")
	}
}
