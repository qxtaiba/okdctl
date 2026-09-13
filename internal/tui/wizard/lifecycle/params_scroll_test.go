package lifecycle

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// resizeParamsStep returns a focused resize params step: sizing (memory,
// vcpus, os disk) then disruption (drain mode, drain timeout).
func resizeParamsStep(t *testing.T) *ParamsStep {
	t.Helper()
	st := &State{Cfg: config.DefaultConfig(), Op: node.OpResize, Scope: node.ResizeScope{Role: nodetypes.RoleMaster}}
	s := NewParamsStep(st)
	_ = s.Init()
	return s
}

func TestParamsStep_FocusedSpanCoversFirstAndLastField(t *testing.T) {
	s := resizeParamsStep(t)

	lines := strings.Split(s.View(86, 24), "\n")
	span, ok := s.FocusedSpan()
	if !ok {
		t.Fatal("FocusedSpan() reported no span for the first field")
	}
	if block := strings.Join(lines[span.Start:span.End+1], "\n"); !strings.Contains(block, "memory (mb)") {
		t.Fatalf("first span %+v = %q, want the memory field", span, block)
	}

	tab := tea.KeyPressMsg{Code: tea.KeyTab}
	for range 4 {
		s.Update(tab)
	}

	lines = strings.Split(s.View(86, 24), "\n")
	span, ok = s.FocusedSpan()
	if !ok {
		t.Fatal("FocusedSpan() reported no span for the last field")
	}
	if span.End >= len(lines) {
		t.Fatalf("last span %+v is outside the %d rendered rows", span, len(lines))
	}
	if block := strings.Join(lines[span.Start:span.End+1], "\n"); !strings.Contains(block, "drain timeout") {
		t.Fatalf("last span %+v = %q, want the drain timeout field", span, block)
	}
}

// paramsBodyRows returns the frame rows between the header and footer rules.
func paramsBodyRows(t *testing.T, frame string) []string {
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

func TestParamsStep_FocusedFieldStaysOnScreen(t *testing.T) {
	st := &State{Cfg: config.DefaultConfig(), Op: node.OpResize, Scope: node.ResizeScope{Role: nodetypes.RoleMaster}}
	m := wizard.NewFlowModel(NewSteps(st, Hooks{}), st.Cfg, lifecycleChrome())

	_ = tuitest.RenderAt(t, m, 90, 24)
	m.Update(wizard.JumpToStepMsg{StepID: StepIDParams})
	frame := tuitest.RenderAt(t, m, 90, 24)
	if !strings.Contains(tuitest.StripANSI(frame), "scroll") {
		t.Fatal("setup: the resize params form does not overflow a 90x24 terminal")
	}

	labels := []string{"vcpus", "os disk (gb)", "drain mode (", "drain timeout"}
	for i, want := range labels {
		m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		m.Update(wizard.FocusChangedMsg{})

		rows := paramsBodyRows(t, m.View().Content)
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
