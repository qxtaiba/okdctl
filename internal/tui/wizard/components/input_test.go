package components

import (
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

func TestInputField_BoxIsExactlyBoxWidth(t *testing.T) {
	for _, outer := range []int{12, 32, 56} {
		f := NewInputField("cluster name", "")
		f.SetWidth(90)
		f.SetBoxWidth(outer)
		rows := strings.Split(tuitest.StripANSI(f.View()), "\n")
		for _, r := range rows[1:4] {
			if lipgloss.Width(r) != outer {
				t.Fatalf("outer %d row %q width %d", outer, r, lipgloss.Width(r))
			}
		}
		if !strings.HasPrefix(rows[1], "╭") || !strings.HasPrefix(rows[3], "╰") {
			t.Fatalf("box rows %q", rows)
		}
	}
}

func TestInputField_ErrorBorderWinsOverFocus(t *testing.T) {
	f := NewInputField("name", "")
	f.Required = true
	f.SetWidth(40)
	_ = f.Focus()
	_ = f.Validate()

	rows := strings.Split(f.View(), "\n")
	borderRow := rows[1]

	errPrefix := ansiPrefix(t, tui.ColorError)
	focusPrefix := ansiPrefix(t, tui.ColorPrimary)

	if !strings.Contains(borderRow, errPrefix) {
		t.Fatalf("border row missing ColorError sequence: %q", borderRow)
	}
	if strings.Contains(borderRow, focusPrefix) {
		t.Fatalf("border row unexpectedly contains ColorPrimary sequence: %q", borderRow)
	}
}

func TestInputField_HelpOnlyWhenFocused(t *testing.T) {
	f := NewInputField("name", "")
	f.Help = "lowercase letters only"
	f.SetWidth(40)

	rows := strings.Split(tuitest.StripANSI(f.View()), "\n")
	if len(rows) != 4 {
		t.Fatalf("blurred rows = %d, want 4: %q", len(rows), rows)
	}

	_ = f.Focus()
	rows = strings.Split(tuitest.StripANSI(f.View()), "\n")
	if len(rows) != 5 {
		t.Fatalf("focused rows = %d, want 5: %q", len(rows), rows)
	}
	if !strings.HasPrefix(rows[4], f.Help) {
		t.Fatalf("last row = %q, want help %q", rows[4], f.Help)
	}
}

func TestInputField_BlurredLongValueShowsHead(t *testing.T) {
	f := NewInputField("name", "")
	f.SetWidth(90)
	f.SetBoxWidth(32)
	value := strings.Repeat("abcdefghij", 6)
	f.SetValue(value)
	f.Blur()

	rows := strings.Split(tuitest.StripANSI(f.View()), "\n")
	want := "│ " + value[:10]
	if !strings.HasPrefix(rows[2], want) {
		t.Fatalf("content row = %q, want prefix %q", rows[2], want)
	}
}

func TestInputField_EmptyShowsDot(t *testing.T) {
	f := NewInputField("name", "")
	f.SetWidth(40)

	rows := strings.Split(tuitest.StripANSI(f.View()), "\n")
	if !strings.Contains(rows[2], "·") {
		t.Fatalf("content row = %q, want a dim ·", rows[2])
	}
}

func TestInputField_NoPromptInsideBox(t *testing.T) {
	f := NewInputField("name", "example")
	f.SetWidth(40)

	rows := strings.Split(tuitest.StripANSI(f.View()), "\n")
	if strings.Contains(rows[2], "> ") {
		t.Fatalf("content row contains a prompt: %q", rows[2])
	}
}

func TestInputField_IsDefaultRendersTag(t *testing.T) {
	f := NewInputField("name", "")
	f.SetValue("mycluster")
	f.SetWidth(90)
	f.isDefault = true

	got := tuitest.StripANSI(f.View())
	if !strings.Contains(got, "default") {
		t.Fatalf("View() = %q, want a default tag", got)
	}
}

// ansiPrefix renders "x" in fg and returns the ANSI escape sequence that
// precedes it, so a test can assert a rendered row was styled with fg.
func ansiPrefix(t *testing.T, fg color.Color) string {
	t.Helper()
	rendered := lipgloss.NewStyle().Foreground(fg).Render("x")
	i := strings.Index(rendered, "x")
	if i < 0 {
		t.Fatalf("rendered marker %q lost its %q", rendered, "x")
	}
	return rendered[:i]
}

func TestInputGroup_ViewMatchesFieldViews(t *testing.T) {
	g := NewInputGroup(
		NewInputField("name", "cluster"),
		NewPasswordField("token", "secret"),
		NewSelectField("approve", []string{"no", "yes"}),
	)
	g.SetWidth(60)

	views := g.FieldViews()
	if len(views) != len(g.Fields()) {
		t.Fatalf("FieldViews() returned %d views, want %d", len(views), len(g.Fields()))
	}
	if got, want := g.View(), strings.Join(views, "\n\n"); got != want {
		t.Fatalf("View() does not equal FieldViews() joined by a blank row:\n got %q\nwant %q", got, want)
	}
}

func TestInputGroup_FieldViewsEmptyGroup(t *testing.T) {
	g := NewInputGroup()
	if views := g.FieldViews(); len(views) != 0 {
		t.Fatalf("FieldViews() = %v, want empty", views)
	}
	if got := g.View(); got != "" {
		t.Fatalf("View() = %q, want empty", got)
	}
}
