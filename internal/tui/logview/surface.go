package logview

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// KeyLock and KeyFull are the log surface's two keys: freeze the window
// where it stands, and swap the log full-screen.
const (
	KeyLock = 'l'
	KeyFull = 'f'
)

// NarrowTailRows is how many log lines ride under a step's own body when the
// frame is too narrow to give the log a pane of its own.
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
	// Recorded heights, one per window mode, stamped by the render methods.
	fullH, tailH, paneW, paneH int
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
// the log full-screen and back — reported through layoutToggled so the
// owning step can ask its frame for a re-measure — pgup/pgdn page the
// window through the whole ring, and the arrows walk it line by line in
// full-screen mode. paneCarries names the window the geometry keys move by:
// the split pane when the frame gives the log one, the tail otherwise. With
// a nil Src every key is inert.
func (s *Surface) HandleKey(msg tea.KeyPressMsg, paneCarries bool) (layoutToggled bool) {
	if s.Src == nil {
		return false
	}
	switch msg.Code {
	case KeyLock:
		s.v.locked = !s.v.locked
		if s.v.locked {
			s.v.lockAt = lockedAt(s.Src)
		}
	case KeyFull:
		s.v.full = !s.v.full
		return true
	case tea.KeyPgUp:
		s.ScrollBy(-s.PageSize(paneCarries), paneCarries)
	case tea.KeyPgDown:
		s.ScrollBy(s.PageSize(paneCarries), paneCarries)
	case tea.KeyUp:
		if s.v.full {
			s.ScrollBy(-1, paneCarries)
		}
	case tea.KeyDown:
		if s.v.full {
			s.ScrollBy(1, paneCarries)
		}
	}
	return false
}

// ConsumesPaging reports whether pgup/pgdn page the log window itself — the
// full-screen log always, a locked pane or tail too — so the frame leaves
// the keys to the step instead of scrolling its own viewport.
func (s *Surface) ConsumesPaging() bool {
	return s.Src != nil && (s.v.full || s.v.locked)
}

// ScrollBy moves the log window n lines through the ring at the geometry
// last rendered, flooring at the stream's oldest full window.
func (s *Surface) ScrollBy(n int, paneCarries bool) {
	w, h, wrap := s.geometry(paneCarries)
	scroll(&s.v, s.Src, n, topLines(s.Src, w, h, wrap))
}

// PageSize is how many lines one pgup/pgdn moves: exactly the lines the
// active window is showing, so a page never skips past unread ones.
func (s *Surface) PageSize(paneCarries bool) int {
	w, h, wrap := s.geometry(paneCarries)
	return visibleLines(s.Src, s.v, w, h, wrap)
}

// geometry names the active log window: the full-screen box, the split
// pane, or the narrow tail under the step's own body.
func (s *Surface) geometry(paneCarries bool) (width, height int, wrap bool) {
	switch {
	case s.v.full:
		return max(s.ViewCol, 1), max(s.fullH, 2), true
	case paneCarries:
		return max(s.paneW, 1), max(s.paneH, 2), false
	default:
		return max(s.ViewCol, 1), max(s.tailH, NarrowTailRows) + 1, false
	}
}

// RenderPane renders the log into a split layout's right pane and records
// the pane geometry the paging keys move by.
func (s *Surface) RenderPane(width, height int) string {
	s.paneW, s.paneH = width, height
	return renderPane(s.Src, s.v, width, height, false)
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
// no source or nothing captured; the caller decides when a pane already
// carries the log instead.
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
