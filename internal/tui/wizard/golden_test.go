package wizard

import (
	"fmt"
	"testing"

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
}
