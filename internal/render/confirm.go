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

// DryRunActions renders the boxed dry-run preview shared by cleanup, destroy, and update-ingress: facts, the would-run actions, and a re-run hint.
func DryRunActions(title string, facts []Fact, would []string) string {
	sb := NewBuilder()
	sb.WriteString("\n")
	sb.WriteString("  " + tui.WarningStyle.Render("dry-run — no changes made") + "\n")
	sb.Newline()

	for _, f := range facts {
		sb.KV(f.Key, f.Value)
	}
	sb.Newline()

	sb.Section("would")
	for _, w := range would {
		sb.Bullet(w)
	}
	sb.Newline()

	sb.WriteString("  " + tui.HighlightStyle.Render(tui.IconPointer+" re-run without --dry-run to execute") + "\n")

	return "\n" + tui.BoxedSectionAccent(sb.String(), title, tui.DefaultBoxWidth, tui.ColorWarning) + "\n"
}
