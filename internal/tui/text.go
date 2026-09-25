package tui

import (
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
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
	runes := []rune(s)
	head := takeWidth(runes, headW)
	tail := takeWidthFromEnd(runes, tailW)
	return string(head) + "…" + string(tail)
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

// takeWidthFromEnd returns the longest suffix of runes whose combined
// lipgloss.Width is at most w; the cut never lands on a bare combining mark,
// since a zero-width mark always "fits" the backward scan on its own and
// would otherwise strand at the front of the suffix once its base rune is
// excluded.
func takeWidthFromEnd(runes []rune, w int) []rune {
	width := 0
	for i := len(runes) - 1; i >= 0; i-- {
		rw := lipgloss.Width(string(runes[i]))
		if width+rw > w {
			return dropLeadingMarks(runes[i+1:])
		}
		width += rw
	}
	return runes
}

// dropLeadingMarks strips combining marks left with no base rune at the
// front of runes.
func dropLeadingMarks(runes []rune) []rune {
	i := 0
	for i < len(runes) && unicode.IsMark(runes[i]) {
		i++
	}
	return runes[i:]
}
