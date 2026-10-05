package wizard

import (
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

type widthOwnerStep struct {
	nopStep
	owns bool
}

func (s *widthOwnerStep) OwnsFrameWidth() bool { return s.owns }

// TestLayoutChangedMsg_ReMeasuresTheBody pins why the message exists: a step
// flipping its own OwnsFrameWidth changes the body width with no terminal
// resize to trigger one.
func TestLayoutChangedMsg_ReMeasuresTheBody(t *testing.T) {
	s := &widthOwnerStep{nopStep: *newNopStep()}
	m := NewFlowModel([]WizardStep{s}, config.DefaultConfig(), DefaultChrome())
	tuitest.RenderAt(t, m, 180, 48)

	capped := m.viewport.Width()
	if capped != singleFormMaxWidth {
		t.Fatalf("capped body width = %d, want singleFormMaxWidth %d", capped, singleFormMaxWidth)
	}

	s.owns = true
	m.Update(LayoutChangedMsg{})

	if got := m.viewport.Width(); got == capped {
		t.Errorf("body width stayed %d after the step claimed the whole frame", got)
	}
	if got, want := m.viewport.Width(), m.contentWidth(); got != want {
		t.Errorf("body width = %d, want the full content width %d", got, want)
	}
}
