package render

import "github.com/qxtaiba/okdctl/internal/tui"

// Fact is one dotted key/value row rendered inside a ConfirmBox.
type Fact struct {
	Key   string
	Value string
}

// ConfirmBox renders the shared confirmation box every destructive command
// prints ahead of its confirmation gate — even under --yes — listing facts
// as dotted rows and turning the box amber, or red with a bold irreversible
// line, depending on whether irreversible is empty.
func ConfirmBox(title string, facts []Fact, irreversible string) string {
	sb := NewBuilder()
	sb.WriteString("\n")
	sb.WriteString("  " + tui.WarningStyle.Render(tui.IconWarning+" confirm "+title) + "\n")
	sb.Newline()

	for _, f := range facts {
		sb.KV(f.Key, f.Value)
	}
	sb.Newline()

	accent := tui.ColorWarning
	if irreversible != "" {
		accent = tui.ColorError
		for _, line := range tui.WrapLines("irreversible — "+irreversible, sb.ContentWidth()) {
			sb.WriteString("  " + tui.ErrorStyle.Render(line) + "\n")
		}
		sb.Newline()
	}

	return "\n" + tui.BoxedSectionAccent(sb.String(), title, tui.DefaultBoxWidth, accent) + "\n"
}
