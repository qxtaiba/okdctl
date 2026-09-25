package components

import (
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// TestRebuildStylesRebindsEveryThemedStyle pins the polarity flip reaching
// the init-captured styles: under the dual-polarity Theme every role
// rebinds, so a rendered field label and the overlay title must carry
// light-polarity SGR after a light flip, not their init-time dark values.
func TestRebuildStylesRebindsEveryThemedStyle(t *testing.T) {
	t.Cleanup(func() {
		tui.SetDarkBackground(true)
		RebuildStyles()
	})

	tui.SetDarkBackground(false)
	RebuildStyles()

	if label := labelStyle.Render("host"); !strings.Contains(label, "51;65;85") {
		t.Errorf("field label = %q, want the light TextSoft tier (#334155)", label)
	}
	if e := errStyle.Render("bad"); !strings.Contains(e, "220;38;38") {
		t.Errorf("error label = %q, want the light Error tier (#DC2626)", e)
	}
	if title := helpOverlayTitleStyle.Render("key bindings"); !strings.Contains(title, "15;23;42") {
		t.Errorf("overlay title = %q, want the light Text tier (#0F172A)", title)
	}
	if hint := helpOverlayHintStyle.Render("esc"); !strings.Contains(hint, "148;163;184") {
		t.Errorf("overlay hint = %q, want the light Subtle tier (#94A3B8)", hint)
	}
}
