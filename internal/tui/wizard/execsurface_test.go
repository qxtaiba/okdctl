package wizard

import (
	"testing"

	"github.com/qxtaiba/okdctl/internal/tui"
)

func TestExecStyleCacheRebuildsOnThemeFlip(t *testing.T) {
	t.Cleanup(func() { tui.SetDarkBackground(true) })
	tui.SetDarkBackground(true)

	var c ExecStyleCache
	dark := c.Styles().Bold.GetForeground()

	tui.SetDarkBackground(false)
	if light := c.Styles().Bold.GetForeground(); light == dark {
		t.Fatalf("Bold stayed %v across the polarity flip", dark)
	}
}

func TestFrameSizeReportsTheBodyBox(t *testing.T) {
	var f FrameSize
	if got := f.BodyHeight(); got != 0 {
		t.Fatalf("BodyHeight before sizing = %d, want 0", got)
	}
	f.SetBodyHeight(31)
	if got := f.BodyHeight(); got != 31 {
		t.Fatalf("BodyHeight = %d, want 31", got)
	}
}
