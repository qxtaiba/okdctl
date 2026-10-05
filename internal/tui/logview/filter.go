package logview

import (
	"sort"
	"strings"
)

// filter is one log window's live line filter: the text the operator typed —
// a leading "!" negating the match — and whether they are still typing it.
// The ring is never touched and never reordered; a filter selects a
// subsequence of one snapshot at render time and nothing more.
type filter struct {
	text   string
	typing bool
}

// active reports whether the filter selects anything; an empty pattern, or a
// bare "!" with nothing to negate, leaves the whole stream showing.
func (f filter) active() bool {
	return f.needle() != ""
}

// needle is the lowercased text a line must carry — or must not, when the
// pattern is negated.
func (f filter) needle() string {
	return strings.ToLower(strings.TrimPrefix(f.text, "!"))
}

// engaged reports whether the operator has a filter in hand at all — one
// committed, or one still being typed. A window with an engaged filter always
// renders its header, even with nothing matching: the chip is the only thing
// on screen that explains the empty rows.
func (f filter) engaged() bool {
	return f.active() || f.typing
}

// negated reports whether the pattern was armed with a leading "!".
func (f filter) negated() bool {
	return strings.HasPrefix(f.text, "!")
}

// matches reports whether l passes the filter, case-insensitively against
// the row's rendered text so its WARN/ERROR tag is filterable too.
func (f filter) matches(l *Line) bool {
	if !f.active() {
		return true
	}
	return strings.Contains(strings.ToLower(lineText(l)), f.needle()) != f.negated()
}

// interesting reports whether l is a line the jump keys stop on: a filter
// match while a filter is live, a warning or error otherwise — so n/N are
// the minimap's companion even with nothing filtered.
func (f filter) interesting(l *Line) bool {
	if f.active() {
		return f.matches(l)
	}
	return levelTag(l.Level) != ""
}

// edit applies one keystroke of filter input: a printable rune extends the
// pattern, backspace shortens it, and the caller commits or cancels.
func (f filter) edit(text string, backspace bool) filter {
	switch {
	case backspace:
		if r := []rune(f.text); len(r) > 0 {
			f.text = string(r[:len(r)-1])
		}
	case text != "":
		f.text += text
	}
	return f
}

// stream is the sequence a window pages through: every line of one snapshot,
// or the subsequence a filter selected. idx is nil while unfiltered, where a
// member's absolute stream index is simply first+i — the common path must not
// allocate an index per line on every frame.
type stream struct {
	lines []Line
	first int64
	idx   []int64
}

// selectFrom returns the sequence f leaves of one snapshot.
func (f filter) selectFrom(lines []Line, first int64) stream {
	if !f.active() {
		return stream{lines: lines, first: first}
	}
	st := stream{first: first}
	for i := range lines {
		if f.matches(&lines[i]) {
			st.lines = append(st.lines, lines[i])
			st.idx = append(st.idx, first+int64(i))
		}
	}
	return st
}

// len is how many members the sequence holds.
func (s *stream) len() int {
	return len(s.lines)
}

// at is the absolute stream index of member i.
func (s *stream) at(i int) int64 {
	if s.idx == nil {
		return s.first + int64(i)
	}
	return s.idx[i]
}

// below counts the members sitting strictly before absolute index a — the
// window's end position in the sequence's own coordinates.
func (s *stream) below(a int64) int {
	if s.idx == nil {
		return min(max(int(a-s.first), 0), len(s.lines))
	}
	return sort.Search(len(s.idx), func(i int) bool { return s.idx[i] >= a })
}
