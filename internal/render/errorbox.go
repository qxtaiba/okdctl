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
// errtypes.Describe (never re-parsed Error() text) for the kind and hint.
func ErrorSummary(err error, exitCode int, runID string) string {
	kind, headline, hint := describeError(err)

	contentWidth := tui.DefaultBoxWidth - 4

	sb := NewBuilder()
	sb.Newline()
	sb.WriteString("  " + tui.ErrorStyle.Render(tui.IconError+"  "+kind) + "\n")
	sb.Newline()
	for _, line := range wrapText(headline, contentWidth) {
		sb.WriteString("  " + line + "\n")
	}
	if hint != "" {
		sb.Newline()
		pointer := tui.HighlightStyle.Render(tui.IconPointer)
		wrapped := wrapText(hint, contentWidth-2)
		for i, line := range wrapped {
			if i == 0 {
				sb.WriteString("  " + pointer + " " + line + "\n")
			} else {
				sb.WriteString("    " + line + "\n")
			}
		}
	}
	sb.Newline()
	footer := fmt.Sprintf("exit %d · run_id %s", exitCode, runID)
	sb.WriteString("  " + tui.MutedStyle.Render(footer) + "\n")

	return "\n" + tui.BoxedSectionAccent(sb.String(), "error", tui.DefaultBoxWidth, tui.ColorError) + "\n"
}

// describeError decomposes err via errtypes.Describe, falling back to a plain
// "error" chip for untyped errors.
func describeError(err error) (kind, headline, hint string) {
	if d, ok := errtypes.Describe(err); ok {
		return d.Kind.Label(), strings.TrimSpace(d.Message), strings.TrimSpace(d.Hint)
	}
	return "error", strings.TrimSpace(err.Error()), ""
}

// wrapText greedy-wraps s to width columns, hard-splitting tokens that exceed
// it; returns at least one line.
func wrapText(s string, width int) []string {
	width = max(width, 1)
	var lines []string
	var cur strings.Builder
	for _, word := range strings.Fields(s) {
		for len(word) > width {
			if cur.Len() > 0 {
				lines = append(lines, cur.String())
				cur.Reset()
			}
			lines = append(lines, word[:width])
			word = word[width:]
		}
		switch {
		case cur.Len() == 0:
			cur.WriteString(word)
		case cur.Len()+1+len(word) <= width:
			cur.WriteByte(' ')
			cur.WriteString(word)
		default:
			lines = append(lines, cur.String())
			cur.Reset()
			cur.WriteString(word)
		}
	}
	if cur.Len() > 0 || len(lines) == 0 {
		lines = append(lines, cur.String())
	}
	return lines
}
