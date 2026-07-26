package render

import (
	"errors"
	"fmt"
	"strings"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/tui"
)

// presented wraps an error whose failure the command has already rendered as a
// rich box (deploy's FailureSummary/InterruptSummary). The top-level handler
// checks IsPresented to avoid stacking a second, generic error box on top.
// Unwrap keeps the chain intact so errors.As exit-code classification still
// walks to the underlying errtypes value.
type presented struct{ err error }

// Presented marks err as already surfaced to the user by a command-owned box.
// Returns nil when err is nil.
func Presented(err error) error {
	if err == nil {
		return nil
	}
	return &presented{err: err}
}

func (p *presented) Error() string { return p.err.Error() }
func (p *presented) Unwrap() error { return p.err }

// IsPresented reports whether err (or anything it wraps) was already rendered
// by a command-owned failure box.
func IsPresented(err error) bool {
	var p *presented
	return errors.As(err, &p)
}

// ErrorSummary renders a command error inside the same boxed chrome the deploy
// summaries use, skinned red. It reads the errtypes kind for the headline,
// wraps the message, promotes the trailing "; <hint>" clause to a next-step
// line, and footers the exit code and run_id for bug reports. This gives an
// ordinary node/status failure the same designed treatment a deploy failure
// already gets, instead of a raw "[ERROR] command failed err=…" log line.
func ErrorSummary(err error, exitCode int, runID string) string {
	kind := errKindLabel(err)
	body := strings.TrimSpace(strings.TrimPrefix(err.Error(), kind+": "))
	headline, hint := splitHint(body)

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

// errKindLabel maps err to the errtypes headline used as the box's kind chip,
// falling back to a plain "error" for untyped failures (exit 1).
func errKindLabel(err error) string {
	var cfgErr *errtypes.ConfigError
	if errors.As(err, &cfgErr) {
		return "config error"
	}
	var netErr *errtypes.NetworkError
	if errors.As(err, &netErr) {
		return "network error"
	}
	var clusterErr *errtypes.ClusterError
	if errors.As(err, &clusterErr) {
		return "cluster error"
	}
	var authErr *errtypes.AuthError
	if errors.As(err, &authErr) {
		return "auth error"
	}
	var usageErr *errtypes.UsageError
	if errors.As(err, &usageErr) {
		return "usage error"
	}
	return "error"
}

// splitHint separates the trailing "; <actionable hint>" clause the errtypes
// messages carry from the headline. When no "; " is present the whole message
// is the headline and hint is empty.
func splitHint(msg string) (headline, hint string) {
	i := strings.LastIndex(msg, "; ")
	if i < 0 {
		return msg, ""
	}
	return strings.TrimSpace(msg[:i]), strings.TrimSpace(msg[i+2:])
}

// wrapText greedy-wraps s to width columns, hard-splitting any single token
// (a long path or terraform address) that exceeds width so the box never
// overflows its budget. Returns at least one line.
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
