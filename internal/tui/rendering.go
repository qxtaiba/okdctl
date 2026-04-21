package tui

// SubsectionLabel renders title with the highlighted subsection style.
func SubsectionLabel(title string) string {
	return HighlightStyle.Render(title)
}

// CompletionSuccess renders msg with the green-check success prefix.
func CompletionSuccess(msg string) string {
	return SuccessStyle.Render(IconSuccess) + " " + TextStyle.Render(msg)
}

// CompletionError renders msg with the red-cross error prefix.
func CompletionError(msg string) string {
	return ErrorStyle.Render(IconError) + " " + TextStyle.Render(msg)
}
