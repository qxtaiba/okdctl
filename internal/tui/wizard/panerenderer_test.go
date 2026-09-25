package wizard

import (
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

// paneOwnerStep fills the split layout's right pane itself, the seam a live log
// beside a running install uses in place of the context pane.
type paneOwnerStep struct {
	nopStep
	content   string
	lastWidth int
	lastRows  int
	suppress  bool
}

func (s *paneOwnerStep) PaneContent(width, height int) string {
	s.lastWidth, s.lastRows = width, height
	return s.content
}

func (s *paneOwnerStep) SuppressesSplit() bool { return s.suppress }

func newPaneOwnerStep(content string) *paneOwnerStep {
	return &paneOwnerStep{nopStep: *newNopStep(), content: content}
}

func TestPaneRenderer_ReplacesTheContextPane(t *testing.T) {
	s := newPaneOwnerStep("LOG\n09:00:01 deploy step started")
	m := NewFlowModel([]WizardStep{s}, config.DefaultConfig(), DefaultChrome())

	frame := tuitest.StripANSI(tuitest.RenderAt(t, m, 180, 48))
	if !strings.Contains(frame, "deploy step started") {
		t.Fatalf("the step's pane content must render:\n%s", frame)
	}
	if strings.Contains(frame, "STEPS") {
		t.Errorf("a step that fills the pane must replace the context pane's step list:\n%s", frame)
	}
	if s.lastWidth < paneMinWidth || s.lastWidth > paneMaxWidth {
		t.Errorf("pane width = %d, want it inside [%d,%d]", s.lastWidth, paneMinWidth, paneMaxWidth)
	}
	if s.lastRows != 48-fixedLayoutOverhead {
		t.Errorf("pane height = %d, want the viewport's %d rows", s.lastRows, 48-fixedLayoutOverhead)
	}
}

// TestPaneRenderer_EmptyContentFallsBackToTheContextPane keeps the pane useful
// on a step that has nothing to show yet, instead of blanking it.
func TestPaneRenderer_EmptyContentFallsBackToTheContextPane(t *testing.T) {
	m := NewFlowModel([]WizardStep{newPaneOwnerStep("")}, config.DefaultConfig(), DefaultChrome())

	frame := tuitest.StripANSI(tuitest.RenderAt(t, m, 180, 48))
	if !strings.Contains(frame, "STEPS") {
		t.Errorf("empty pane content must fall back to the context pane:\n%s", frame)
	}
}

// TestPaneRenderer_NotConsultedBelowTheSplit proves the narrow tier never asks
// for pane content it has nowhere to put.
func TestPaneRenderer_NotConsultedBelowTheSplit(t *testing.T) {
	s := newPaneOwnerStep("LOG\n09:00:01 line")
	m := NewFlowModel([]WizardStep{s}, config.DefaultConfig(), DefaultChrome())

	frame := tuitest.StripANSI(tuitest.RenderAt(t, m, 149, 48))
	if strings.Contains(frame, "09:00:01") {
		t.Errorf("pane content leaked into an unsplit frame:\n%s", frame)
	}
}

func TestSplitsFrameMatchesTheFramesOwnGate(t *testing.T) {
	cases := []struct {
		w, h, steps int
		want        bool
	}{
		{180, 48, 2, true},
		{150, 13, 2, true},
		{149, 48, 2, false},
		{180, 12, 2, false},
	}
	for _, c := range cases {
		if got := SplitsFrame(c.w, c.h, c.steps); got != c.want {
			t.Errorf("SplitsFrame(%d,%d,%d) = %v, want %v", c.w, c.h, c.steps, got, c.want)
		}
	}
}

// TestLayoutChangedMsg_ReMeasuresTheBody pins why the message exists: a step
// flipping its own SuppressesSplit changes the body width with no terminal
// resize to trigger one.
func TestLayoutChangedMsg_ReMeasuresTheBody(t *testing.T) {
	s := newPaneOwnerStep("LOG\n09:00:01 line")
	m := NewFlowModel([]WizardStep{s}, config.DefaultConfig(), DefaultChrome())
	tuitest.RenderAt(t, m, 180, 48)

	split := m.viewport.Width()
	if split != formMaxWidth {
		t.Fatalf("split body width = %d, want formMaxWidth %d", split, formMaxWidth)
	}

	s.suppress = true
	m.Update(LayoutChangedMsg{})

	if got := m.viewport.Width(); got == split {
		t.Errorf("body width stayed %d after the step claimed the whole frame", got)
	}
	if got, want := m.viewport.Width(), m.contentWidth(); got != want {
		t.Errorf("body width = %d, want the full content width %d", got, want)
	}
}
