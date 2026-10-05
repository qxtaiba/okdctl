package components

import (
	"errors"
	"image/color"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

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

	errPrefix := ansiPrefix(t, tui.ColorError())
	focusPrefix := ansiPrefix(t, tui.ColorPrimary())

	if !strings.Contains(borderRow, errPrefix) {
		t.Fatalf("border row missing ColorError() sequence: %q", borderRow)
	}
	if strings.Contains(borderRow, focusPrefix) {
		t.Fatalf("border row unexpectedly contains ColorPrimary() sequence: %q", borderRow)
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

func TestInputField_UndoRestoresValueAtFocus(t *testing.T) {
	f := NewInputField("host", "")
	f.SetWidth(60)
	f.SetValue("pve-current")
	f.Focus()
	f.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	f.Update(tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl})
	if got := f.Value(); got != "pve-current" {
		t.Fatalf("undo = %q, want value at focus", got)
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

func TestInputField_PlaceholderStyleDiffersFromDefaultValueStyle(t *testing.T) {
	placeholderField := NewInputField("repository", "ssh://git@example.com/org/repo.git")
	placeholderField.SetWidth(60)
	placeholderRow := strings.Split(placeholderField.View(), "\n")[2]

	defaultField := NewInputField("branch", "")
	defaultField.SetWidth(60)
	defaultField.SetDefault("main")
	defaultRow := strings.Split(defaultField.View(), "\n")[2]

	// Un-stripped: the placeholder's hint text renders italic (a standalone
	// SGR 3 parameter — "\x1b[3;" preceding a combined code, or a lone
	// "\x1b[3m") on top of its own dimmer color, while an unmodified
	// default's text carries no italic — the two must never collapse into
	// the same look, or an empty box reads as already-filled until a
	// submit-time validation error. (Checking for "\x1b[3" alone would
	// false-positive on the unrelated 256/24-bit color codes "38"/"3" + a
	// digit that both rows also carry.)
	hasItalic := func(row string) bool {
		return strings.Contains(row, "\x1b[3;") || strings.Contains(row, "\x1b[3m")
	}
	if !hasItalic(placeholderRow) {
		t.Fatalf("placeholder row has no italic SGR code: %q", placeholderRow)
	}
	if hasItalic(defaultRow) {
		t.Fatalf("default-value row unexpectedly carries an italic SGR code: %q", defaultRow)
	}

	// Stripped, the two must still read as clearly different: the
	// placeholder shows its hint text, the default shows its real value
	// plus the " default" tag.
	strippedPlaceholder := tuitest.StripANSI(placeholderField.View())
	strippedDefault := tuitest.StripANSI(defaultField.View())
	if !strings.Contains(strippedPlaceholder, "ssh://git@example.com/org/repo.git") {
		t.Fatalf("placeholder view lost its hint text: %q", strippedPlaceholder)
	}
	if !strings.Contains(strippedDefault, "default") {
		t.Fatalf("default view lost its \" default\" tag: %q", strippedDefault)
	}
}

func TestInputField_DisabledDimsBorderBeyondUnfocused(t *testing.T) {
	enabled := NewInputField("drain timeout", "10m")
	enabled.SetWidth(60)
	enabled.SetValue("10m")

	disabled := NewInputField("drain timeout", "10m")
	disabled.SetWidth(60)
	disabled.SetValue("10m")
	disabled.Disabled = true

	enabledRows := strings.Split(enabled.View(), "\n")
	disabledRows := strings.Split(disabled.View(), "\n")

	// The border row must differ (a dimmer color) from the ordinary
	// unfocused state, or a disabled field looks identical to any other
	// blurred field — the whole point is a visibly different affordance.
	if enabledRows[1] == disabledRows[1] {
		t.Fatalf("disabled top-border row is identical to an ordinary unfocused field: %q", disabledRows[1])
	}
	focused := NewInputField("drain timeout", "10m")
	focused.SetWidth(60)
	focused.SetValue("10m")
	_ = focused.Focus()
	focusedRows := strings.Split(focused.View(), "\n")
	if disabledRows[1] == focusedRows[1] {
		t.Fatalf("disabled border must never render the same as a focused border: %q", disabledRows[1])
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

// TestInputField_DefaultPasteToReplace pins bracketed paste to the same
// contract as typing: a paste over an unmodified default replaces the text
// and drops the default tag instead of merging into it.
func TestInputField_DefaultPasteToReplace(t *testing.T) {
	f := NewInputField("username", "")
	f.SetDefault("root@pam")
	_ = f.Focus()

	f.Update(tea.PasteMsg{Content: "admin@pve"})

	if got := f.Value(); got != "admin@pve" {
		t.Fatalf("Value() after pasting over a default = %q, want %q", got, "admin@pve")
	}
	if f.IsDefault() {
		t.Fatal("IsDefault() after a paste = true, want false")
	}
}

func TestInputField_PasteClearsStaleError(t *testing.T) {
	f := NewInputField("username", "")
	f.Required = true
	_ = f.Focus()
	if f.Validate() == nil {
		t.Fatal("Validate() on an empty required field = nil, want an error")
	}

	f.Update(tea.PasteMsg{Content: "admin@pve"})

	if f.err != nil {
		t.Fatalf("err after a paste = %v, want cleared", f.err)
	}
}

// TestInputField_PasteRejectsEmbeddedNewline pins the reconciliation-audit
// defect where a pasted multi-line secret (e.g. an SSH key) was forwarded
// straight to bubbles' textinput, which silently collapses embedded
// newlines into spaces and accepts the mangled result with no error.
func TestInputField_PasteRejectsEmbeddedNewline(t *testing.T) {
	f := NewInputField("ssh key", "")
	f.SetValue("original-value")
	_ = f.Focus()

	const secret = "-----BEGIN KEY-----\nMIIFAKEKEYDATA\n-----END KEY-----"
	f.Update(tea.PasteMsg{Content: secret})

	if got := f.Value(); got != "original-value" {
		t.Fatalf("Value() after a rejected multi-line paste = %q, want the original value preserved", got)
	}
	if f.err == nil {
		t.Fatal("err after a rejected multi-line paste = nil, want a rejection error")
	}
	for _, fragment := range []string{"BEGIN KEY", "MIIFAKEKEYDATA", "END KEY"} {
		if strings.Contains(f.err.Error(), fragment) {
			t.Fatalf("err = %q, must not echo any fragment of the rejected paste", f.err.Error())
		}
	}
}

// TestInputField_PasteRejectsControlByte covers a pasted path carrying a
// stray control byte, the other half of the audit's reproduction.
func TestInputField_PasteRejectsControlByte(t *testing.T) {
	f := NewInputField("path", "")
	f.SetValue("original-value")
	_ = f.Focus()

	f.Update(tea.PasteMsg{Content: "/etc/pass\x00wd"})

	if got := f.Value(); got != "original-value" {
		t.Fatalf("Value() after a rejected control-byte paste = %q, want the original value preserved", got)
	}
	if f.err == nil {
		t.Fatal("err after a rejected control-byte paste = nil, want a rejection error")
	}
}

// TestInputField_PasteRejectionClearsOnNextValidPaste proves a rejection is
// not sticky: a following well-formed paste is accepted and clears the
// rejection error, matching the field's existing error-clear-on-edit rule.
func TestInputField_PasteRejectionClearsOnNextValidPaste(t *testing.T) {
	f := NewInputField("ssh key", "")
	_ = f.Focus()

	f.Update(tea.PasteMsg{Content: "line one\nline two"})
	if f.err == nil {
		t.Fatal("err after the rejected multi-line paste = nil, want a rejection error")
	}

	f.Update(tea.PasteMsg{Content: "ssh-ed25519 AAAA"})
	if f.err != nil {
		t.Fatalf("err after a valid paste following a rejection = %v, want cleared", f.err)
	}
	if got := f.Value(); got != "ssh-ed25519 AAAA" {
		t.Fatalf("Value() after a valid paste following a rejection = %q, want it accepted", got)
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

// sgrStatesOf returns the accumulated SGR state preceding each visible rune
// of needle within row, so a test can assert a run of text renders in one
// uniform style.
func sgrStatesOf(t *testing.T, row, needle string) []string {
	t.Helper()

	var visible []rune
	var states []string
	state := ""
	for i := 0; i < len(row); {
		if strings.HasPrefix(row[i:], "\x1b[") {
			end := strings.IndexByte(row[i:], 'm')
			if end < 0 {
				t.Fatalf("unterminated SGR sequence in %q", row)
			}
			seq := row[i : i+end+1]
			if seq == "\x1b[0m" || seq == "\x1b[m" {
				state = ""
			} else {
				state += seq
			}
			i += end + 1
			continue
		}
		r, size := utf8.DecodeRuneInString(row[i:])
		visible = append(visible, r)
		states = append(states, state)
		i += size
	}

	vis := string(visible)
	idx := strings.Index(vis, needle)
	if idx < 0 {
		t.Fatalf("row %q does not contain %q", vis, needle)
	}
	start := utf8.RuneCountInString(vis[:idx])
	return states[start : start+utf8.RuneCountInString(needle)]
}

func assertOneUniformStyle(t *testing.T, row, needle string) {
	t.Helper()
	states := sgrStatesOf(t, row, needle)
	if states[0] == "" {
		t.Fatalf("first rune of %q carries no style at all in %q", needle, row)
	}
	for i, st := range states {
		if st != states[0] {
			t.Fatalf("rune %d of %q styled %q, want %q as rune 0", i, needle, st, states[0])
		}
	}
}

func TestInputField_BlurredDefaultValueOneUniformStyle(t *testing.T) {
	f := NewInputField("host", "")
	f.SetWidth(60)
	f.SetDefault("192.168.1.100:8006")
	f.Blur()

	assertOneUniformStyle(t, strings.Split(f.View(), "\n")[2], "192.168.1.100:8006")
}

func TestInputField_BlurredTypedValueOneUniformStyle(t *testing.T) {
	f := NewInputField("username", "")
	f.SetWidth(60)
	f.SetValue("root@pam")
	f.Blur()

	assertOneUniformStyle(t, strings.Split(f.View(), "\n")[2], "root@pam")
}

func TestInputField_BlurredPlaceholderOneUniformStyle(t *testing.T) {
	f := NewInputField("token id", "root@pam!okdctl")
	f.SetWidth(60)
	f.Blur()

	assertOneUniformStyle(t, strings.Split(f.View(), "\n")[2], "root@pam!okdctl")
}

func TestCompactPasswordRendering(t *testing.T) {
	f := NewPasswordField("password", "optional")
	f.SetWidth(60)
	f.Help = "Preserve CASE in help"
	f.SetValue("SensitiveValue")
	inactive := f.View()
	if strings.Contains(inactive, "SensitiveValue") || strings.Contains(inactive, f.Help) {
		t.Fatal("inactive field leaks secret or shows help")
	}
	f.Focus()
	active := f.View()
	if strings.Contains(active, "SensitiveValue") || !strings.Contains(active, f.Help) {
		t.Fatal("focused password or help incorrect")
	}
	if lipgloss.Height(inactive) >= lipgloss.Height(active) {
		t.Fatal("inactive field is not compact")
	}
	f.Validator = func(string) error { return errors.New("Cannot read /Mixed/CASE: SensitiveValue") }
	if err := f.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
	out := f.View()
	if strings.Contains(out, "SensitiveValue") || !strings.Contains(out, "/Mixed/CASE") {
		t.Fatal("error lost case or exposed secret")
	}
}

func TestCursorMotionDoesNotRepeatValidation(t *testing.T) {
	f := NewInputField("path", "")
	f.SetValue("/fixture")
	f.Focus()
	calls := 0
	f.Validator = func(string) error { calls++; return nil }
	f.Valid()
	f.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	f.Valid()
	if calls != 1 {
		t.Fatalf("cursor movement reran validator: %d", calls)
	}
	f.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	f.Valid()
	if calls != 2 {
		t.Fatalf("edit did not rerun validator: %d", calls)
	}
}
