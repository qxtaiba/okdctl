package tui

// SubsectionLabel renders title with the highlighted subsection style.
func SubsectionLabel(title string) string {
	return Downsample(HighlightStyle.Render(title))
}

// CompletionSuccess renders msg with the green-check success prefix.
func CompletionSuccess(msg string) string {
	return Downsample(SuccessStyle.Render(IconSuccess) + " " + TextStyle.Render(msg))
}

// CompletionError renders msg with the red-cross error prefix.
func CompletionError(msg string) string {
	return Downsample(ErrorStyle.Render(IconError) + " " + TextStyle.Render(msg))
}
