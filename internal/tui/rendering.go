package tui

// SubsectionLabel renders title in the highlight style used for subsection
// headings inside wizard panels and status output.
func SubsectionLabel(title string) string {
	return HighlightStyle.Render(title)
}

// CompletionSuccess renders msg prefixed with the success icon.
func CompletionSuccess(msg string) string {
	return SuccessStyle.Render(IconSuccess) + " " + TextStyle.Render(msg)
}

// CompletionError renders msg prefixed with the error icon.
func CompletionError(msg string) string {
	return ErrorStyle.Render(IconError) + " " + TextStyle.Render(msg)
}
