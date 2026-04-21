package tui

func SubsectionLabel(title string) string {
	return HighlightStyle.Render(title)
}

func CompletionSuccess(msg string) string {
	return SuccessStyle.Render(IconSuccess) + " " + TextStyle.Render(msg)
}

func CompletionError(msg string) string {
	return ErrorStyle.Render(IconError) + " " + TextStyle.Render(msg)
}
