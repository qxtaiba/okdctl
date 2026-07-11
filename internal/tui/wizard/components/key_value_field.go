package components

import (
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
)

var errKVEmptyKey = errors.New("key cannot be empty")

// KeyValueField renders an editable table of key=value rows.
// Navigate mode: j/k moves the row cursor; h/l switches the active column;
// a adds a row; d deletes the current row. ctrl+e enters edit mode on the
// active cell; pressing ctrl+e again commits and exits edit mode.
// enter, tab, and shift+tab are consumed by the host DataDrivenStep for
// inter-field navigation and therefore cannot be used as edit-commit keys —
// same constraint as MultiSelectField and SelectField.
type KeyValueField struct {
	Label     string
	Help      string
	Validator func(string) error

	rows     []kvRow
	cursor   int
	col      int
	editMode bool
	focused  bool
	err      error
	width    int
}

type kvRow struct {
	keyInput textinput.Model
	valInput textinput.Model
}

func newKVRow(k, v string) kvRow {
	ki := textinput.New()
	ki.CharLimit = 128
	ki.SetWidth(20)
	ki.SetValue(k)

	vi := textinput.New()
	vi.CharLimit = 128
	vi.SetWidth(20)
	vi.SetValue(v)

	return kvRow{keyInput: ki, valInput: vi}
}

// NewKeyValueField returns a KeyValueField with one empty placeholder row.
func NewKeyValueField(label string) *KeyValueField {
	return &KeyValueField{
		Label: label,
		rows:  []kvRow{newKVRow("", "")},
	}
}

// Value serializes rows as "k1=v1,k2=v2". Rows with a blank key are omitted.
// Values containing a comma will not round-trip through SetValue.
func (f *KeyValueField) Value() string {
	var parts []string
	for i := range f.rows {
		r := &f.rows[i]
		k := r.keyInput.Value()
		v := r.valInput.Value()
		if strings.TrimSpace(k) == "" {
			continue
		}
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, ",")
}

// SetValue parses a "k1=v1,k2=v2" string into rows, replacing any current
// content. Values containing a comma will not round-trip through Value.
func (f *KeyValueField) SetValue(value string) {
	f.rows = parseKVString(value)
	if len(f.rows) == 0 {
		f.rows = []kvRow{newKVRow("", "")}
	}
	f.cursor = 0
	f.col = 0
	f.editMode = false
}

// Focus gives the field keyboard focus and returns the textinput blink
// command for the active cell (when in edit mode).
func (f *KeyValueField) Focus() tea.Cmd {
	f.focused = true
	return f.syncInputFocus()
}

// Blur removes focus and exits edit mode.
func (f *KeyValueField) Blur() {
	f.focused = false
	f.editMode = false
	f.blurAllInputs()
}

// SetWidth records the available rendering width and resizes each row's
// textinputs so key and value columns fill the available space evenly.
func (f *KeyValueField) SetWidth(width int) {
	f.width = width
	half := (width - 8) / 2
	if half < 10 {
		half = 10
	}
	for i := range f.rows {
		f.rows[i].keyInput.SetWidth(half)
		f.rows[i].valInput.SetWidth(half)
	}
}

// Validate rejects rows with a non-empty value but empty key, then runs the
// field Validator against the serialized Value if one is set.
func (f *KeyValueField) Validate() error {
	for i := range f.rows {
		r := &f.rows[i]
		if strings.TrimSpace(r.keyInput.Value()) == "" && r.valInput.Value() != "" {
			f.err = errKVEmptyKey
			return f.err
		}
	}
	if f.Validator != nil {
		f.err = f.Validator(f.Value())
		return f.err
	}
	f.err = nil
	return nil
}

// Update routes messages: ctrl+e toggles edit mode; in navigate mode
// j/k/h/l/a/d adjust cursor and rows; in edit mode all other keystrokes
// forward to the active textinput.
func (f *KeyValueField) Update(msg tea.Msg) (FormField, tea.Cmd) {
	if !f.focused {
		return f, nil
	}
	if keyMsg, isKey := msg.(tea.KeyPressMsg); isKey {
		if key.Matches(keyMsg, key.NewBinding(key.WithKeys("ctrl+e"))) {
			cmd := f.toggleEditMode()
			return f, cmd
		}
		if !f.editMode {
			return f.updateNavigate(keyMsg)
		}
	}
	if f.editMode {
		return f.updateEdit(msg)
	}
	return f, nil
}

func (f *KeyValueField) toggleEditMode() tea.Cmd {
	if f.editMode {
		f.editMode = false
		f.blurAllInputs()
		return nil
	}
	if len(f.rows) == 0 {
		f.rows = []kvRow{newKVRow("", "")}
	}
	f.editMode = true
	return f.syncInputFocus()
}

func (f *KeyValueField) updateEdit(msg tea.Msg) (FormField, tea.Cmd) {
	if len(f.rows) == 0 {
		return f, nil
	}
	var cmd tea.Cmd
	r := &f.rows[f.cursor]
	if f.col == 0 {
		r.keyInput, cmd = r.keyInput.Update(msg)
	} else {
		r.valInput, cmd = r.valInput.Update(msg)
	}
	return f, cmd
}

func (f *KeyValueField) updateNavigate(keyMsg tea.KeyPressMsg) (FormField, tea.Cmd) {
	switch {
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("k"))):
		if f.cursor > 0 {
			f.cursor--
		}
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("j"))):
		if f.cursor < len(f.rows)-1 {
			f.cursor++
		}
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("h"))):
		f.col = 0
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("l"))):
		f.col = 1
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("a"))):
		cmd := f.addRow()
		return f, cmd
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("d"))):
		f.deleteRow()
	}
	return f, nil
}

func (f *KeyValueField) addRow() tea.Cmd {
	f.rows = append(f.rows, newKVRow("", ""))
	f.cursor = len(f.rows) - 1
	f.col = 0
	f.editMode = true
	cmd := f.syncInputFocus()
	if f.width > 0 {
		f.SetWidth(f.width)
	}
	return cmd
}

func (f *KeyValueField) deleteRow() {
	if len(f.rows) == 0 {
		return
	}
	newRows := make([]kvRow, 0, len(f.rows)-1)
	newRows = append(newRows, f.rows[:f.cursor]...)
	newRows = append(newRows, f.rows[f.cursor+1:]...)
	f.rows = newRows
	if f.cursor >= len(f.rows) && f.cursor > 0 {
		f.cursor--
	}
}

func (f *KeyValueField) syncInputFocus() tea.Cmd {
	f.blurAllInputs()
	if !f.editMode || len(f.rows) == 0 {
		return nil
	}
	if f.col == 0 {
		return f.rows[f.cursor].keyInput.Focus()
	}
	return f.rows[f.cursor].valInput.Focus()
}

func (f *KeyValueField) blurAllInputs() {
	for i := range f.rows {
		f.rows[i].keyInput.Blur()
		f.rows[i].valInput.Blur()
	}
}

// View renders the label, column header, all data rows, and any validation
// error. The active row shows live textinputs when in edit mode.
func (f *KeyValueField) View() string {
	labelStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate300)
	hintStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate500)

	labelLine := labelStyle.Render(strings.ToLower(f.Label))
	if f.Help != "" {
		labelLine += " " + hintStyle.Render("("+strings.ToLower(f.Help)+")")
	}
	if f.focused && f.editMode {
		labelLine += " " + hintStyle.Render("(editing — ctrl+e done)")
	} else if f.focused {
		labelLine += " " + hintStyle.Render("(j/k rows · h/l col · a add · d del · ctrl+e edit)")
	}

	colW := f.cellWidth()
	lines := []string{labelLine, f.viewHeader(colW)}
	for i := range f.rows {
		lines = append(lines, f.viewRow(i, colW))
	}
	if f.err != nil {
		errStyle := lipgloss.NewStyle().Foreground(tui.ColorError)
		lines = append(lines, errStyle.Render("✖ "+strings.ToLower(f.err.Error())))
	}
	return strings.Join(lines, "\n")
}

func (f *KeyValueField) cellWidth() int {
	w := (f.width - 8) / 2
	if w < 10 {
		return 10
	}
	return w
}

func (f *KeyValueField) viewHeader(colW int) string {
	hdrStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate500)
	return hdrStyle.Render(fmt.Sprintf("  %-*s  %-*s", colW, "key", colW, "value"))
}

func (f *KeyValueField) viewRow(i, colW int) string {
	cursorStyle := lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true)
	activeStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate300)
	dimStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate500)
	boxFocus := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(tui.ColorPrimary).
		Padding(0, 1).Width(colW)
	boxIdle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(tui.ColorSlate600).
		Padding(0, 1).Width(colW)

	r := &f.rows[i]
	isCursor := f.focused && i == f.cursor
	cur := "  "
	if isCursor {
		cur = cursorStyle.Render("> ")
	}

	if f.editMode && isCursor {
		if f.col == 0 {
			return cur + boxFocus.Render(r.keyInput.View()) + "  " + boxIdle.Render(r.valInput.View())
		}
		return cur + boxIdle.Render(r.keyInput.View()) + "  " + boxFocus.Render(r.valInput.View())
	}

	kVal := fmt.Sprintf("%-*s", colW, r.keyInput.Value())
	vVal := fmt.Sprintf("%-*s", colW, r.valInput.Value())
	if isCursor && f.col == 0 {
		return cur + activeStyle.Render(kVal) + "  " + dimStyle.Render(vVal)
	}
	if isCursor && f.col == 1 {
		return cur + dimStyle.Render(kVal) + "  " + activeStyle.Render(vVal)
	}
	return cur + dimStyle.Render(kVal) + "  " + dimStyle.Render(vVal)
}

func parseKVString(value string) []kvRow {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	var rows []kvRow
	for _, pair := range strings.Split(value, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		eq := strings.IndexByte(pair, '=')
		if eq < 0 {
			rows = append(rows, newKVRow(pair, ""))
			continue
		}
		rows = append(rows, newKVRow(
			strings.TrimSpace(pair[:eq]),
			strings.TrimSpace(pair[eq+1:]),
		))
	}
	return rows
}
