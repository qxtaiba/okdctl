package components

import (
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

func TestLabeledField_EveryFieldKindReportsItsLabelAndHelp(t *testing.T) {
	input := NewInputField("host", "192.168.1.1")
	input.Help = "proxmox host"

	sel := NewSelectField("provider", []string{"a", "b"})
	sel.Help = "pick a provider"

	multi := NewMultiSelectField("addons", []string{"a", "b"})
	multi.Help = "enable addons"

	kv := NewKeyValueField("labels")
	kv.Help = "extra labels"

	fields := []LabeledField{input, sel, multi, kv}
	wantLabels := []string{"host", "provider", "addons", "labels"}
	wantHelp := []string{"proxmox host", "pick a provider", "enable addons", "extra labels"}

	for i, f := range fields {
		if got := f.FieldLabel(); got != wantLabels[i] {
			t.Errorf("field %d FieldLabel() = %q, want %q", i, got, wantLabels[i])
		}
		if got := f.FieldHelp(); got != wantHelp[i] {
			t.Errorf("field %d FieldHelp() = %q, want %q", i, got, wantHelp[i])
		}
	}
}

func TestInputField_BoxIsExactlyBoxWidth(t *testing.T) {
	for _, outer := range []int{16, 40, 64} {
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

// TestInputField_RequiredErrorNamesFix pins the R4 fix for the wizard's
// most-triggered error: a blank required field must say what to do, AND
// which field, not a generic "this field is required".
func TestInputField_RequiredErrorNamesFix(t *testing.T) {
	f := NewInputField("cluster name", "")
	f.Required = true

	const want = "cluster name is required — enter a value"
	if err := f.Check(); err == nil || err.Error() != want {
		t.Fatalf("Check() = %v, want %q", err, want)
	}
}

// TestInputField_RequiredErrorFallsBackWithoutLabel covers the defensive
// branch: a Required field constructed without a label must not render
// "is required" with an empty subject.
func TestInputField_RequiredErrorFallsBackWithoutLabel(t *testing.T) {
	f := NewInputField("", "")
	f.Required = true

	const want = "this field is required — enter a value"
	if err := f.Check(); err == nil || err.Error() != want {
		t.Fatalf("Check() = %v, want %q", err, want)
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

func TestInputField_FocusedEmptyNoPlaceholderHidesDot(t *testing.T) {
	f := NewInputField("name", "")
	f.SetWidth(40)
	_ = f.Focus()

	rows := strings.Split(tuitest.StripANSI(f.View()), "\n")
	if strings.Contains(rows[2], "·") {
		t.Fatalf("content row = %q, want no dim dot while focused (a real cursor instead)", rows[2])
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

func TestInputField_SavedPositionSurvivesRedundantBlur(t *testing.T) {
	a := NewInputField("a", "")
	b := NewInputField("b", "")
	c := NewInputField("c", "")
	g := NewInputGroup(a, b, c)
	g.SetWidth(60)

	_ = g.Focus()
	a.SetValue("abcdefghij")
	a.input.SetCursor(5)

	g.Next() // a -> b: a's real blur saves position 5
	g.Next() // b -> c: a's redundant re-blur must not collapse it to 0
	g.Next() // c -> a (wraps): a's Focus restores the saved position

	if got := a.input.Position(); got != 5 {
		t.Fatalf("Position() after refocus = %d, want 5 (saved position lost to a redundant Blur)", got)
	}
}

func TestInputField_BlurUntouchedDoesNotValidate(t *testing.T) {
	f := NewInputField("name", "")
	f.Required = true

	f.Blur()

	if f.err != nil {
		t.Fatalf("Blur() on a never-focused field set err = %v, want nil", f.err)
	}
}

func TestInputField_BlurTouchedValidates(t *testing.T) {
	f := NewInputField("name", "")
	f.Required = true
	_ = f.Focus()

	f.Blur()

	if f.err == nil {
		t.Fatal("Blur() on a focused-then-blurred field did not validate")
	}
}

func TestInputField_DefaultTypeToReplace(t *testing.T) {
	f := NewInputField("cluster name", "")
	f.SetDefault("mycluster")
	_ = f.Focus()

	f.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})

	if got := f.Value(); got != "h" {
		t.Fatalf("Value() after typing over a default = %q, want %q", got, "h")
	}
	if f.IsDefault() {
		t.Fatal("IsDefault() after typing = true, want false")
	}
}

func TestInputField_DefaultArrowKeepsText(t *testing.T) {
	f := NewInputField("cluster name", "")
	f.SetDefault("mycluster")
	_ = f.Focus()

	f.Update(tea.KeyPressMsg{Code: tea.KeyRight})

	if got := f.Value(); got != "mycluster" {
		t.Fatalf("Value() after an arrow key = %q, want the default text preserved", got)
	}
	if !f.IsDefault() {
		t.Fatal("IsDefault() after an arrow key = false, want true (pure navigation keeps the default tag)")
	}
}

// TestInputField_DefaultTagFitsAtPathWidthNarrowAvail pins the
// secretstore_op_connect_host regression: a Path-width (64) field with a
// default must reserve room for the " default" tag out of its available
// width, so box+tag join into exactly avail columns instead of overflowing
// it by the tag's width.
func TestInputField_DefaultTagFitsAtPathWidthNarrowAvail(t *testing.T) {
	f := NewInputField("connect host", "")
	f.SetBoxWidth(64) // wizard.FieldWidthPath
	f.SetWidth(70)
	f.SetDefault("http://onepassword-connect:8080")

	rows := strings.Split(tuitest.StripANSI(f.View()), "\n")
	contentRow := rows[2]
	if got := lipgloss.Width(contentRow); got != 70 {
		t.Fatalf("box+tag row width = %d, want 70 (62-wide box + 8-wide tag, attached): %q", got, contentRow)
	}
	if !strings.Contains(contentRow, "default") {
		t.Fatalf("content row = %q, want the default tag attached on the same row as the box", contentRow)
	}
}

// TestInputField_BoxWidthStableAcrossDefaultTagDrop pins geometry
// stability: a field that has ever carried a default reserves the tag's
// room for its whole life (hasDefault, permanent), so the box itself never
// widens when the user's first keystroke drops the (now-cleared) default
// tag — only isDefault clears; boxOuterWidth must not. A reserve keyed off
// isDefault instead would pass TestInputField_DefaultTagFitsAtPathWidthNarrowAvail
// but fail here: the box would jump from 62 to 70 wide the instant the tag
// drops.
func TestInputField_BoxWidthStableAcrossDefaultTagDrop(t *testing.T) {
	f := NewInputField("connect host", "")
	f.SetBoxWidth(64)
	f.SetWidth(70)
	f.SetDefault("http://onepassword-connect:8080")
	_ = f.Focus()

	before := f.boxOuterWidth()

	f.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	if f.IsDefault() {
		t.Fatal("IsDefault() after typing = true, want false")
	}

	after := f.boxOuterWidth()
	if before != after {
		t.Fatalf("box width before typing = %d, after the default tag dropped = %d, want unchanged", before, after)
	}
}

func TestInputField_SetValueClearsDefaultTag(t *testing.T) {
	f := NewInputField("cluster name", "")
	f.SetDefault("mycluster")

	f.SetValue("homelab")

	if f.IsDefault() {
		t.Fatal("IsDefault() after SetValue = true, want false")
	}
	if got := f.Value(); got != "homelab" {
		t.Fatalf("Value() after SetValue = %q, want homelab", got)
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
