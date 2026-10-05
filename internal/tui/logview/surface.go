package logview

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// The log surface's keys: freeze the window where it stands, swap the log
// full-screen, open the filter input, and step the window between lines of
// interest — the next filter match while a filter is live, the next warning
// or error otherwise.
const (
	KeyLock      = 'l'
	KeyFull      = 'f'
	KeyFilter    = '/'
	KeyNextMatch = 'n'
	KeyPrevMatch = 'N'
)

// NarrowTailRows is the fewest log lines that ride under a step's own body.
const NarrowTailRows = 6

// Surface is one screen's log viewport: the source it reads, the
// lock/full-screen state, and the geometry of whichever window was last
// rendered, so a paging key always moves by exactly the window the operator
// is looking at.
type Surface struct {
	// Src is the log stream the surface reads; nil leaves every key and
	// render inert, since an empty full-screen log would blank the body.
	Src Source
	// ViewCol is the column budget of the step's own body as last rendered —
	// the width the full-screen and tail windows draw at. The owning step
	// records it at the top of its View.
	ViewCol int

	v view
	// committed is the filter that was live before the input opened, restored
	// when the operator escapes out of it.
	committed filter
	// Recorded heights, one per window mode, stamped by the render methods.
	fullH, tailH int
}

// Full reports whether the log has taken the whole frame.
func (s *Surface) Full() bool {
	return s.v.full
}

// Locked reports whether the window is frozen rather than following the tail.
func (s *Surface) Locked() bool {
	return s.v.locked
}

// LockPoint reports the absolute stream index the locked window ends at
// (exclusive); zero while following.
func (s *Surface) LockPoint() int64 {
	return s.v.lockAt
}

// HandleKey applies one of the log surface's keys: KeyLock freezes the
// window where it stands (or releases it back to the tail), KeyFull swaps
// the log full-screen and back — reported through relayout so the owning
// step can ask its frame for a re-measure — pgup/pgdn page the window
// through the whole ring, and the arrows walk it line by line in
// full-screen mode. moved reports that KeyLock or a jump (KeyNextMatch,
// KeyPrevMatch) moved the window without any layout change: on the narrow
// tail the window rides below the step's own body, so the owning step must
// still nudge the outer viewport toward it, or the newly-locked or
// newly-jumped-to line can sit off screen below the fold. With a nil Src
// every key is inert.
func (s *Surface) HandleKey(msg tea.KeyPressMsg) (relayout, moved bool) {
	if s.Src == nil {
		return false, false
	}
	if s.v.filter.typing {
		s.editFilter(msg)
		return false, false
	}
	switch {
	case letterKey(msg, KeyLock):
		s.v.locked = !s.v.locked
		if s.v.locked {
			s.v.lockAt = lockedAt(s.Src)
		}
		return false, true
	case letterKey(msg, KeyFull):
		s.v.full = !s.v.full
		return true, false
	case letterKey(msg, KeyFilter):
		// The pattern starts empty on every open: filter-as-you-type counts
		// its matches from the first keystroke, and re-opening to edit a long
		// pattern is not the gesture — re-typing a short one is.
		s.committed, s.v.filter = s.v.filter, filter{typing: true}
	case letterKey(msg, KeyNextMatch):
		jump(&s.v, s.Src, 1)
		return false, true
	case letterKey(msg, KeyPrevMatch):
		jump(&s.v, s.Src, -1)
		return false, true
	case msg.Code == tea.KeyPgUp:
		s.ScrollBy(-s.PageSize())
	case msg.Code == tea.KeyPgDown:
		s.ScrollBy(s.PageSize())
	case msg.Code == tea.KeyUp:
		if s.v.full {
			s.ScrollBy(-1)
		}
	case msg.Code == tea.KeyDown:
		if s.v.full {
			s.ScrollBy(1)
		}
	}
	return false, false
}

// letterKey reports whether msg is the printable key r. Text is the field a
// shifted letter arrives in, so it decides whenever the terminal filled it;
// a terminal that reports only Code falls back to that.
func letterKey(msg tea.KeyPressMsg, r rune) bool {
	if msg.Text != "" {
		return msg.Text == string(r)
	}
	return msg.Code == r
}

// editFilter applies one keystroke of filter input: enter commits the
// pattern, esc restores whatever was live before the input opened, backspace
// shortens, and any printable text extends. Nothing else moves the window —
// while the input is open the surface owns every key it is handed.
func (s *Surface) editFilter(msg tea.KeyPressMsg) {
	switch msg.Code {
	case tea.KeyEnter:
		s.v.filter.typing = false
	case tea.KeyEsc:
		s.v.filter = s.committed
	case tea.KeyBackspace:
		s.v.filter = s.v.filter.edit("", true)
	default:
		if msg.Text != "" {
			s.v.filter = s.v.filter.edit(msg.Text, false)
		}
	}
}

// Filtering reports whether the filter input is open and taking typed text,
// which is what makes the frame hand the surface its scroll and help keys.
func (s *Surface) Filtering() bool {
	return s.Src != nil && s.v.filter.typing
}

// CancelFilter closes an open filter input, restoring the pattern that was
// live before it opened, and reports whether there was one to close — the esc
// a forward-only step would otherwise swallow.
func (s *Surface) CancelFilter() bool {
	if !s.Filtering() {
		return false
	}
	s.v.filter = s.committed
	return true
}

// Filtered reports whether a committed filter is selecting the window's rows.
func (s *Surface) Filtered() bool {
	return s.Src != nil && s.v.filter.active()
}

// ConsumesPaging reports whether pgup/pgdn page the log window itself — the
// full-screen log always, a locked tail too, and an open filter input
// where they must stand still — so the frame leaves the keys to the step
// instead of scrolling its own viewport.
func (s *Surface) ConsumesPaging() bool {
	return s.Src != nil && (s.v.full || s.v.locked || s.v.filter.typing)
}

// ScrollBy moves the log window n lines through the ring at the geometry
// last rendered, flooring at the stream's oldest full window.
func (s *Surface) ScrollBy(n int) {
	w, h, wrap := s.geometry()
	scroll(&s.v, s.Src, n, topLines(s.Src, s.v, w, h, wrap))
}

// PageSize is how many lines one pgup/pgdn moves: exactly the lines the
// active window is showing, so a page never skips past unread ones.
func (s *Surface) PageSize() int {
	w, h, wrap := s.geometry()
	return visibleLines(s.Src, s.v, w, h, wrap)
}

// geometry names the active log window: the full-screen box or the tail
// under the step's own body.
func (s *Surface) geometry() (width, height int, wrap bool) {
	if s.v.full {
		return max(s.ViewCol, 1), max(s.fullH, 2), true
	}
	return max(s.ViewCol, 1), max(s.tailH, NarrowTailRows) + 1, false
}

// RenderTail renders the rows that ride under the step's own body — the
// same stamped rows, led by the dim LOG header — and records the tail
// budget; nil when the ring is still empty.
func (s *Surface) RenderTail(width, budget int) []string {
	s.tailH = budget
	return renderTail(s.Src, s.v, width, budget)
}

// RenderFull renders the log across the whole body once KeyFull has swapped
// it full-screen, recording the height the paging keys move by.
func (s *Surface) RenderFull(width, height int) string {
	s.fullH = height
	return renderFull(s.Src, s.v, width, height)
}

// FailureTail renders the log's last lines for the region under an error
// card: a failure's evidence is the chatter that led up to it. Empty with
// no source or nothing captured.
func (s *Surface) FailureTail(col int) string {
	if s.Src == nil {
		return ""
	}
	tail := s.RenderTail(col, NarrowTailRows)
	if len(tail) == 0 {
		return ""
	}
	return strings.Join(tail, "\n")
}

// LockHelp names what KeyLock does next, not what it did, so a help ribbon
// reads as an instruction in either state.
func (s *Surface) LockHelp() string {
	if s.v.locked {
		return "follow the log tail"
	}
	return "lock the log here"
}

// FilterHelp names what KeyFilter does next: commit the pattern being typed,
// replace a committed one, or open the input on an unfiltered window.
func (s *Surface) FilterHelp() string {
	switch {
	case s.v.filter.typing:
		return "commit the filter"
	case s.v.filter.active():
		return "filter the log again"
	default:
		return "filter the log"
	}
}

// JumpHelp names what KeyNextMatch and KeyPrevMatch stop on, which depends on
// whether a filter is live: its matches, or the run's warnings and errors.
func (s *Surface) JumpHelp() string {
	if s.v.filter.active() {
		return "next/prev match"
	}
	return "next/prev warn or error"
}

// FullHelp names what KeyFull does next; see LockHelp.
func (s *Surface) FullHelp() string {
	if s.v.full {
		return "back to checklist"
	}
	return "full-screen log"
}

// SinkLine renders the dim pointer naming the run-log file that keeps every
// byte the ring evicts, wrapped at col; empty when no sink is open — a
// screen must never point at a file that does not exist.
func SinkLine(path string, col int) string {
	if path == "" {
		return ""
	}
	dim := lipgloss.NewStyle().Foreground(tui.ColorTextFaint())
	return dim.Render(lipgloss.Wrap("full log "+path, col, ""))
}
