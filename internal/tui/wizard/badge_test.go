package wizard

import (
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

func TestFooterBadgeKeepsTheFullVersionAtEightyColumns(t *testing.T) {
	m := NewFlowModel([]WizardStep{newNopStep()}, config.DefaultConfig(), DefaultChrome())
	frame := tuitest.StripANSI(tuitest.RenderAt(t, m, 80, 24))
	if want := "▸ okd " + config.DefaultOKDVersion + " "; !strings.Contains(frame, want) {
		t.Fatalf("80x24 footer clips the version badge, want %q:\n%s", want, frame)
	}
}

func TestFooterBadgeEllipsizesInsteadOfClipping(t *testing.T) {
	chrome := DefaultChrome()
	chrome.Badge = func(*config.Config) string { return strings.Repeat("x", 70) }
	m := NewFlowModel([]WizardStep{newNopStep()}, config.DefaultConfig(), chrome)
	frame := tuitest.RenderAt(t, m, 80, 24)
	tuitest.AssertFits(t, frame, 80, 24)
	plain := tuitest.StripANSI(frame)
	if !strings.Contains(plain, "…") || strings.Contains(plain, strings.Repeat("x", 40)) {
		t.Fatalf("an over-long badge must be ellipsized to its budget, not clipped or overflowed:\n%s", plain)
	}
}
