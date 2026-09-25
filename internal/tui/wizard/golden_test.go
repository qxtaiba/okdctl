package wizard

import (
	"errors"
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

func TestGolden_ChromeOnly(t *testing.T) {
	sizes := []struct {
		w, h int
		fits bool
	}{
		{80, 24, true},
		{100, 30, true},
		{120, 40, true},
	}

	for _, sz := range sizes {
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.Cluster.Name = "homelab"
			m := NewFlowModel([]WizardStep{newNopStep()}, cfg, DefaultChrome())
			frame := tuitest.RenderAt(t, m, sz.w, sz.h)
			tuitest.Golden(t, fmt.Sprintf("chrome_%dx%d", sz.w, sz.h), frame)
			if sz.fits {
				tuitest.AssertFits(t, frame, sz.w, sz.h)
			}
		})
	}

	t.Run("error", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Cluster.Name = "homelab"
		m := NewFlowModel([]WizardStep{newNopStep()}, cfg, DefaultChrome())
		tuitest.RenderAt(t, m, 100, 30)
		m.Update(ErrorSetMsg{Error: errors.New("boom")})
		frame := m.View().Content
		tuitest.Golden(t, "chrome_error_100x30", frame)
		tuitest.AssertFits(t, frame, 100, 30)
	})
}

// TestGolden_HelpOverlayWideSplit pins the overlay's modal claim on the
// split tier: centered over the full content width, no pane beside it.
func TestGolden_HelpOverlayWideSplit(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Cluster.Name = "homelab"
	m := NewFlowModel([]WizardStep{newNopStep()}, cfg, DefaultChrome())
	_ = tuitest.RenderAt(t, m, 180, 48)

	mm, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	frame := mm.(*Model).View().Content
	tuitest.Golden(t, "help_overlay_180x48", frame)
	tuitest.AssertFits(t, frame, 180, 48)
}
