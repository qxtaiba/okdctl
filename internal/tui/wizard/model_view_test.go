package wizard

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

func TestModel_FrameWidthIsTerminalMinusFour(t *testing.T) {
	for _, w := range []int{80, 100, 120} {
		m := NewModel([]WizardStep{newNopStep()}, config.DefaultConfig())
		frame := tuitest.StripANSI(tuitest.RenderAt(t, m, w, 30))
		for i, line := range strings.Split(strings.TrimRight(frame, "\n"), "\n") {
			// Every row — including the blank outer-padding rows — renders at
			// exactly the terminal width; the P0 bug drew an 86-wide frame at w=80.
			if lw := lipgloss.Width(line); lw != w {
				t.Errorf("w=%d row %d width %d: %q", w, i, lw, line)
			}
			if i == 1 && !strings.HasPrefix(line, "  ╭") {
				t.Errorf("row 1 = %q", line)
			}
		}
	}
}

func TestModel_TooSmallRendersNotice(t *testing.T) {
	m := NewModel([]WizardStep{newNopStep()}, config.DefaultConfig())
	for _, sz := range [][2]int{{59, 24}, {80, 19}} {
		frame := tuitest.StripANSI(tuitest.RenderAt(t, m, sz[0], sz[1]))
		if !strings.Contains(frame, tooSmallNotice) || strings.Contains(frame, "╭") {
			t.Errorf("%v: %q", sz, frame)
		}
	}
}

func TestModel_ViewportHeightBudget(t *testing.T) {
	m := NewModel([]WizardStep{newNopStep()}, config.DefaultConfig())
	for _, sz := range [][2]int{{80, 24}, {100, 30}, {120, 40}} {
		tuitest.RenderAt(t, m, sz[0], sz[1])
		if got := m.viewport.Height(); got != sz[1]-fixedLayoutOverhead {
			t.Errorf("%v viewport height %d", sz, got)
		}
	}
}

func TestModel_StatusRowIsBudgeted(t *testing.T) {
	for _, sz := range [][2]int{{80, 24}, {100, 30}, {120, 40}} {
		m := NewModel([]WizardStep{newNopStep()}, config.DefaultConfig())
		before := strings.Count(tuitest.RenderAt(t, m, sz[0], sz[1]), "\n")
		m.Update(ErrorSetMsg{Error: errors.New("boom")})
		after := strings.Count(m.View().Content, "\n")
		if before != after {
			t.Fatalf("%v: rows %d → %d", sz, before, after)
		}
		if !strings.Contains(tuitest.StripANSI(m.View().Content), "✗ boom") {
			t.Fatalf("%v: status row missing", sz)
		}
	}
}

func TestModel_StatusRowClearsOnKeypress(t *testing.T) {
	m := NewModel([]WizardStep{newNopStep()}, config.DefaultConfig())
	tuitest.RenderAt(t, m, 100, 30)
	m.Update(ErrorSetMsg{Error: errors.New("boom")})
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if strings.Contains(tuitest.StripANSI(m.View().Content), "boom") {
		t.Fatal("status row did not clear")
	}
}

func TestModel_StatusRowTruncatesLongError(t *testing.T) {
	for _, sz := range [][2]int{{80, 24}, {100, 30}, {120, 40}} {
		m := NewModel([]WizardStep{newNopStep()}, config.DefaultConfig())
		tuitest.RenderAt(t, m, sz[0], sz[1])
		m.Update(ErrorSetMsg{Error: errors.New(strings.Repeat("x", 300))})
		tuitest.AssertFits(t, m.View().Content, sz[0], sz[1])
	}
}
