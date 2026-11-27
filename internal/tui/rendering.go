package tui

// CompletionSuccess renders msg with the green-check success prefix.
func CompletionSuccess(msg string) string {
	return Downsample(SuccessStyle.Render(IconSuccess) + " " + TextStyle.Render(msg))
}

// CompletionError renders msg with the red-cross error prefix.
func CompletionError(msg string) string {
	return Downsample(ErrorStyle.Render(IconError) + " " + TextStyle.Render(msg))
}

// EmptyState renders a dim pending glyph, msg and an optional muted hint.
func EmptyState(msg, hint string) string {
	line := DimStyle.Render(IconPending) + " " + TextStyle.Render(msg)
	if hint != "" {
		line += "  " + MutedStyle.Render(hint)
	}
	return Downsample(line)
}
