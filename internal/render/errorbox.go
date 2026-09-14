package render

import (
	"errors"
	"fmt"
	"strings"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/tui"
)

// presented wraps an error already rendered as a rich box; Unwrap keeps
// errors.As reaching the underlying errtypes value.
type presented struct{ err error }

// Presented marks err as already surfaced by a command-owned box; nil in, nil out.
func Presented(err error) error {
	if err == nil {
		return nil
	}
	return &presented{err: err}
}

func (p *presented) Error() string { return p.err.Error() }
func (p *presented) Unwrap() error { return p.err }

// IsPresented reports whether err (or anything it wraps) was already rendered
// by a command-owned box.
func IsPresented(err error) bool {
	var p *presented
	return errors.As(err, &p)
}

// ErrorSummary renders err in the deploy-summary boxed chrome, using
// errtypes.Describe (never re-parsed Error() text) for the kind and hint,
// plus an exit-code/run-id footer.
func ErrorSummary(err error, exitCode int, runID string) string {
	kind, headline, hint := describeError(err)

	sb := errorBody(kind, headline, hint, tui.DefaultBoxWidth)
	sb.Newline()
	footer := fmt.Sprintf("exit %d · run_id %s", exitCode, runID)
	sb.WriteString("  " + tui.MutedStyle.Render(footer) + "\n")

	return "\n" + tui.BoxedSectionAccent(sb.String(), "error", tui.DefaultBoxWidth, tui.ColorError) + "\n"
}

// ErrorCard renders the red error box body — kind chip, wrapped message, and
// pointer-led hint — without the exit-code/run-id footer ErrorSummary adds.
func ErrorCard(kind, message, hint string, width int) string {
	sb := errorBody(kind, message, hint, width)
	return "\n" + tui.BoxedSectionAccent(sb.String(), "error", width, tui.ColorError) + "\n"
}

// errorBody writes the kind chip, wrapped message, and pointer-led hint
// shared by ErrorSummary and ErrorCard, sized to fit inside width.
func errorBody(kind, message, hint string, width int) *Builder {
	sb := NewBuilderWidth(width)
	contentWidth := sb.ContentWidth()
	sb.Newline()
	sb.WriteString("  " + tui.ErrorStyle.Render(tui.IconError+"  "+kind) + "\n")
	sb.Newline()
	for _, line := range tui.WrapLines(message, contentWidth) {
		sb.WriteString("  " + line + "\n")
	}
	if hint != "" {
		sb.Newline()
		pointer := tui.HighlightStyle.Render(tui.IconPointer)
		wrapped := tui.WrapLines(hint, contentWidth-2)
		for i, line := range wrapped {
			if i == 0 {
				sb.WriteString("  " + pointer + " " + line + "\n")
			} else {
				sb.WriteString("    " + line + "\n")
			}
		}
	}
	return sb
}

// describeError decomposes err via errtypes.Describe, falling back to a plain
// "error" chip for untyped errors.
func describeError(err error) (kind, headline, hint string) {
	if d, ok := errtypes.Describe(err); ok {
		return d.Kind.Label(), strings.TrimSpace(d.Message), strings.TrimSpace(d.Hint)
	}
	return "error", strings.TrimSpace(err.Error()), ""
}
