package steps

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// addonFieldLabels flattens the addons definition into the tab order the
// wizard walks, rendering each label the way the field itself does: text,
// password, and select fields show their authored-case label alone, while
// multi-select/key-value fields still fold their help into the label row.
func addonFieldLabels() []string {
	var labels []string
	for si := range AddonsStepDefinition.Sections {
		fields := AddonsStepDefinition.Sections[si].Fields
		for fi := range fields {
			def := &fields[fi]
			switch def.Type {
			case wizard.FieldTypeText, wizard.FieldTypePassword, wizard.FieldTypeSelect:
				labels = append(labels, def.Label)
			default:
				label := strings.ToLower(def.Label)
				if help := strings.ToLower(def.Help); help != "" {
					label += " (" + help[:min(20, len(help))]
				}
				labels = append(labels, label)
			}
		}
	}
	return labels
}

// bodyRows returns the frame rows between the header and footer rules.
func bodyRows(t *testing.T, frame string) []string {
	t.Helper()
	lines := strings.Split(tuitest.StripANSI(frame), "\n")
	first, last := -1, -1
	for i, l := range lines {
		if strings.Contains(l, "│─") {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 || last <= first {
		t.Fatalf("frame has no header/footer rules:\n%s", strings.Join(lines, "\n"))
	}
	rows := make([]string, 0, last-first-1)
	for _, l := range lines[first+1 : last] {
		rows = append(rows, strings.TrimSpace(strings.Trim(strings.TrimSpace(l), "│")))
	}
	return rows
}

func TestAddonsStep_FocusedFieldStaysOnScreen(t *testing.T) {
	m := newGoldenModel(t)
	_ = tuitest.RenderAt(t, m, 80, 24)
	m.Update(wizard.JumpToStepMsg{StepID: wizard.StepIDAddons})
	_ = tuitest.RenderAt(t, m, 80, 24)

	labels := addonFieldLabels()
	for i := range 12 {
		m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		m.Update(wizard.FocusChangedMsg{})

		rows := bodyRows(t, m.View().Content)
		want := labels[i+1]
		at := -1
		for j, row := range rows {
			if strings.HasPrefix(row, want) {
				at = j
				break
			}
		}
		if at < 0 {
			t.Fatalf("tab %d: focused field %q is not on screen:\n%s", i, want, strings.Join(rows, "\n"))
		}
		if len(rows)-at < 4 {
			t.Fatalf("tab %d: focused field %q sits %d rows from the bottom, its box is sliced", i, want, len(rows)-at)
		}
	}
}
