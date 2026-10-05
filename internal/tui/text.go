package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// WrapLines reformats text as space-joined words wrapped to width, breaking
// at spaces only — never inside hyphenated tokens, which would corrupt the
// copy-paste commands and paths this helper wraps — and hard-splitting a
// word wider than a whole line; it never returns an empty slice.
func WrapLines(text string, width int) []string {
	width = max(width, 1)
	var lines []string
	cur, curW := "", 0
	flush := func() {
		lines = append(lines, cur)
		cur, curW = "", 0
	}
	for _, word := range strings.Fields(text) {
		wordW := lipgloss.Width(word)
		if wordW > width {
			if curW > 0 {
				flush()
			}
			runes := []rune(word)
			for len(runes) > 0 {
				head := takeWidth(runes, width)
				if len(head) == 0 {
					head = runes[:1]
				}
				runes = runes[len(head):]
				if len(runes) > 0 {
					lines = append(lines, string(head))
				} else {
					cur, curW = string(head), lipgloss.Width(string(head))
				}
			}
			continue
		}
		sep := 0
		if curW > 0 {
			sep = 1
		}
		if curW+sep+wordW > width {
			flush()
			sep = 0
		}
		if sep == 1 {
			cur += " "
		}
		cur += word
		curW += sep + wordW
	}
	flush()
	return lines
}

// PromptLine styles text as an interactive prompt, leading with the
// highlighted pointer glyph and trailing with a colon and space.
func PromptLine(text string) string {
	return Downsample(HighlightStyle.Render(IconPointer) + " " + text + ": ")
}

// Footnote renders text as a dim explanatory note, downsampled for the
// active color profile.
func Footnote(text string) string {
	return Downsample(MutedStyle.Render(text))
}

// truncateMiddle shortens s to maxW columns by replacing the middle with an
// ellipsis, preserving the distinguishing head and tail; maxW <= 0 returns s
// unchanged.
func truncateMiddle(s string, maxW int) string {
	if maxW <= 0 || lipgloss.Width(s) <= maxW {
		return s
	}
	if maxW <= 1 {
		return "…"
	}
	keep := maxW - 1
	headW, tailW := keep/2, keep-keep/2
	s = foldInvalidUTF8(s)
	return ansi.Truncate(s, headW, "") + "…" + takeWidthFromEnd(s, tailW)
}

// foldInvalidUTF8 replaces each invalid byte with U+FFFD: the ansi truncation
// helpers copy invalid bytes through, even from beyond the cut.
func foldInvalidUTF8(s string) string {
	return string([]rune(s))
}

// takeWidth returns the longest prefix of runes whose combined lipgloss.Width
// is at most w.
func takeWidth(runes []rune, w int) []rune {
	width := 0
	for i, r := range runes {
		rw := lipgloss.Width(string(r))
		if width+rw > w {
			return runes[:i]
		}
		width += rw
	}
	return runes
}

// takeWidthFromEnd returns the longest suffix of s at most w columns wide.
func takeWidthFromEnd(s string, w int) string {
	drop := ansi.StringWidth(s) - w
	tail := ansi.TruncateLeft(s, drop, "")
	// TruncateLeft keeps a wide cell that straddles the cut; drop that cell too.
	if ansi.StringWidth(tail) > w {
		tail = ansi.TruncateLeft(s, drop+1, "")
	}
	return tail
}
