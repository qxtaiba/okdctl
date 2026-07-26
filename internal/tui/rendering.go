package tui

// SubsectionLabel renders title with the highlighted subsection style.
func SubsectionLabel(title string) string {
	return downsample(HighlightStyle.Render(title))
}

// CompletionSuccess renders msg with the green-check success prefix.
func CompletionSuccess(msg string) string {
	return downsample(SuccessStyle.Render(IconSuccess) + " " + TextStyle.Render(msg))
}

// CompletionError renders msg with the red-cross error prefix.
func CompletionError(msg string) string {
	return downsample(ErrorStyle.Render(IconError) + " " + TextStyle.Render(msg))
}

// EmptyState renders a reassuring empty-state line: a dim pending glyph, the
// message, and an optional muted hint — instead of a bare indented string.
func EmptyState(msg, hint string) string {
	line := DimStyle.Render(IconPending) + " " + TextStyle.Render(msg)
	if hint != "" {
		line += "  " + MutedStyle.Render(hint)
	}
	return downsample(line)
}
