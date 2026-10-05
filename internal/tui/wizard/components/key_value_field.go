package components

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// KeyValueField renders an editable key=value table (j/k row, h/l col, a
// add, d delete, ctrl+e edit). tab/shift+tab are reserved by the host
// DataDrivenStep; enter commits the active edit (via EnterConsumer) and only
// submits the step from navigate mode.
type KeyValueField struct {
	Label     string
	Help      string
	Validator func(string) error

	rows     []kvRow
	cursor   int
	col      int
	editMode bool
	// editOriginal is the cell's value when edit mode was entered, restored
	// on escape so a cancelled edit never leaves a partial keystroke behind.
	editOriginal string
	focused      bool
	err          error
	width        int
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

// ConsumesTextInput reports true only mid-edit: navigate mode's keys
// (j/k/h/l/a/d/ctrl+e) are all single-purpose commands, so a "?" there is
// inert and free for the wizard's help-overlay toggle.
func (f *KeyValueField) ConsumesTextInput() bool {
	return f.focused && f.editMode
}

// ConsumesEnter reports true mid-edit, when enter commits the cell edit
// rather than submitting the step.
func (f *KeyValueField) ConsumesEnter() bool {
	return f.focused && f.editMode
}

// Value serializes rows as "k1=v1,k2=v2", omitting rows with a blank key.
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
	half := f.cellWidth()
	for i := range f.rows {
		f.rows[i].keyInput.SetWidth(half)
		f.rows[i].valInput.SetWidth(half)
	}
}

// Check rejects rows with a non-empty value but empty key, then runs the
// field Validator against the serialized Value if one is set.
func (f *KeyValueField) Check() error {
	for i := range f.rows {
		r := &f.rows[i]
		if strings.TrimSpace(r.keyInput.Value()) == "" && r.valInput.Value() != "" {
			label := f.Label
			if label == "" {
				return errors.New("key cannot be empty — enter a key")
			}
			return fmt.Errorf("%s key cannot be empty — enter a key", label)
		}
	}
	if f.Validator != nil {
		return f.Validator(f.Value())
	}
	return nil
}

// Validate runs Check and records the result as the field's current error for View to render.
func (f *KeyValueField) Validate() error {
	f.err = f.Check()
	return f.err
}

// KeyHints returns the field's footer hints, differing between edit and
// navigate mode.
func (f *KeyValueField) KeyHints() []KeyHint {
	if f.editMode {
		return []KeyHint{{Key: "enter/ctrl+e", Help: "done"}}
	}
	return []KeyHint{
		{Key: "j/k", Help: "row"},
		{Key: "h/l", Help: "column"},
		{Key: "a", Help: "add"},
		{Key: "d", Help: "delete"},
		{Key: "ctrl+e", Help: "edit"},
	}
}

// OwnsKey reserves cell commit and cancel while an editor is active, so the
// wizard's own Back/navigation handling never intercepts them.
func (f *KeyValueField) OwnsKey(msg tea.KeyPressMsg) bool {
	return f.editMode && (msg.Code == tea.KeyEnter || msg.Code == tea.KeyEscape)
}

// Editing reports whether typing currently changes a table cell.
func (f *KeyValueField) Editing() bool { return f.editMode }

// Update routes messages: ctrl+e toggles edit mode, enter commits it, and
// escape cancels it by restoring the cell's pre-edit value; navigate mode
// uses j/k/h/l/a/d; edit mode otherwise forwards keys to the active
// textinput.
func (f *KeyValueField) Update(msg tea.Msg) (FormField, tea.Cmd) {
	if !f.focused {
		return f, nil
	}
	if keyMsg, isKey := msg.(tea.KeyPressMsg); isKey {
		if key.Matches(keyMsg, key.NewBinding(key.WithKeys("ctrl+e"))) {
			cmd := f.toggleEditMode()
			return f, cmd
		}
		if f.editMode && keyMsg.Code == tea.KeyEscape {
			if len(f.rows) > 0 {
				if f.col == 0 {
					f.rows[f.cursor].keyInput.SetValue(f.editOriginal)
				} else {
					f.rows[f.cursor].valInput.SetValue(f.editOriginal)
				}
			}
			f.editMode = false
			f.blurAllInputs()
			return f, nil
		}
		if f.editMode && key.Matches(keyMsg, key.NewBinding(key.WithKeys("enter"))) {
			f.editMode = false
			f.blurAllInputs()
			return f, nil
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
	if f.col == 0 {
		f.editOriginal = f.rows[f.cursor].keyInput.Value()
	} else {
		f.editOriginal = f.rows[f.cursor].valInput.Value()
	}
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
	f.rows = slices.Delete(f.rows, f.cursor, f.cursor+1)
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

// View renders the field's card — one row per pair plus an add-row
// trailer — and an error or help row below it.
func (f *KeyValueField) View() string {
	colW := f.cellWidth()

	rows := make([]string, 0, len(f.rows)+1)
	for i := range f.rows {
		rows = append(rows, f.viewRow(i, colW))
	}
	rows = append(rows, f.viewAddRow())

	accent := tui.ColorSubtle()
	switch {
	case f.err != nil:
		accent = tui.ColorError()
	case f.focused:
		accent = tui.ColorPrimary()
	}

	out := tui.Card(f.Label, strings.Join(rows, "\n"), f.width, accent)
	switch {
	case f.err != nil:
		out += "\n" + errStyle.Render(tui.IconError+" "+f.err.Error())
	case f.focused && f.Help != "":
		out += "\n" + helpStyle.Width(f.width).Render(f.Help)
	}
	return out
}

// cellWidth splits the card's inner width between the key and value
// columns, reserving 1 column for the cursor prefix and 2 for the gap
// between them.
func (f *KeyValueField) cellWidth() int {
	inner := f.width - 2
	return max((inner-3)/2, 10)
}

// viewRow renders row i as "key  value", dim unless it holds the cursor; in
// edit mode the cursor row becomes two joined fieldBox cells instead.
func (f *KeyValueField) viewRow(i, colW int) string {
	cursorStyle := lipgloss.NewStyle().Foreground(tui.ColorPrimary()).Bold(true)
	activeStyle := lipgloss.NewStyle().Foreground(tui.ColorTextSoft())
	dimStyle := lipgloss.NewStyle().Foreground(tui.ColorTextFaint())

	r := &f.rows[i]
	isCursor := f.focused && i == f.cursor

	if f.editMode && isCursor {
		focusedKey := f.col == 0
		keyCell, valCell := r.keyInput.View(), kvBlurredCell(&r.valInput, colW)
		if !focusedKey {
			keyCell, valCell = kvBlurredCell(&r.keyInput, colW), r.valInput.View()
		}
		keyBox := fieldBox(keyCell, colW, focusedKey, false, false)
		valBox := fieldBox(valCell, colW, !focusedKey, false, false)
		// JoinHorizontal zips the boxes' rows together; "+" concatenation
		// would instead glue keyBox's last row to valBox's first row.
		return lipgloss.JoinHorizontal(lipgloss.Top, keyBox, "  ", valBox)
	}

	prefix, style := " ", dimStyle
	if isCursor {
		prefix, style = cursorStyle.Render(">"), activeStyle
	}
	content := fmt.Sprintf("%-*s  %-*s", colW, r.keyInput.Value(), colW, r.valInput.Value())
	return prefix + style.Render(content)
}

// kvBlurredCell renders an unfocused edit cell's prompt and value head
// directly in the shared blurred text style, bypassing the textinput's
// cursor path — its parked cursor cell renders through the cursor's own
// TextStyle (never set on the value path), reading as one odd character in
// a uniform cell, and its scroll window can hide the value's head: the same
// family InputField.blurredValueView works around.
func kvBlurredCell(in *textinput.Model, colW int) string {
	style := fieldInputStyles(false, false).Blurred.Text
	budget := max(colW-4-lipgloss.Width(in.Prompt), 1)
	return style.Render(in.Prompt + tui.Truncate(in.Value(), budget))
}

// viewAddRow renders the trailing "+ add" row that the 'a' key acts on.
func (f *KeyValueField) viewAddRow() string {
	return lipgloss.NewStyle().Foreground(tui.ColorTextFaint()).Render(" + add")
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
