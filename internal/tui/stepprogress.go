package tui

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/qxtaiba/okdctl/internal/distribution"
)

// StepMeta identifies one planned deploy step for the checklist: its stable
// ID, display name, and phase label. deploy builds this list from the phases
// that will actually run, so the checklist counter (N/total) reflects the
// current run — a resume that skips setup/install seeds a shorter plan.
type StepMeta struct {
	ID    distribution.StepID
	Name  string
	Phase string
}

// StepProgress renders a live, rewriting deploy step checklist to the terminal
// and mirrors each step to the log sink. It implements
// distribution.MetricsRecorder: StepStarted paints a dim in-progress line,
// StepFinished rewrites it in place with the duration and a ✔/✖/skip glyph and
// commits it to scrollback. Steps that are skipped or already-done emit
// StepFinished without a preceding StepStarted; the renderer tolerates that.
//
// StepProgress is the second line owner (see lineowner.go) alongside the
// spinner/status line a step body spawns. Only one owns the terminal line at a
// time: StepStarted registers the checklist, a spawned spinner/status line
// replaces it as owner for the step's long work, and StepFinished commits the
// final line whether or not the checklist is still the registered owner. The
// install-monitor status line (which repaints live operator/CSR counts via its
// own ticker) is the owner during the monitor step; the checklist's dim line
// for that step is transient hand-off state, so the two never fight.
type StepProgress struct {
	w       io.Writer
	logSink io.Writer
	total   int
	index   map[distribution.StepID]stepPos

	mu sync.Mutex
}

type stepPos struct {
	n    int
	meta StepMeta
}

// NewStepProgress builds a checklist recorder rendering to stderr. logSink is
// the persistent log file (may be nil); per-step records are mirrored there so
// okdctl.log retains the numbered step trail the TTY checklist replaces. Wire
// it only when ProgressBarsEnabled — the non-TTY path leaves the orchestrator's
// own step log lines in place.
func NewStepProgress(plan []StepMeta, logSink io.Writer) *StepProgress {
	return newStepProgress(plan, os.Stderr, logSink)
}

func newStepProgress(plan []StepMeta, w, logSink io.Writer) *StepProgress {
	index := make(map[distribution.StepID]stepPos, len(plan))
	for i, m := range plan {
		index[m.ID] = stepPos{n: i + 1, meta: m}
	}
	return &StepProgress{w: w, logSink: logSink, total: len(plan), index: index}
}

// SuppressStepLog reports that the orchestrator's own per-step Info lines are
// redundant while this recorder renders the checklist, so they demote to Debug
// on the TTY. The per-step trail still reaches okdctl.log via writeSink.
func (s *StepProgress) SuppressStepLog() bool { return true }

// StepStarted paints the dim in-progress line for id and takes ownership of
// the terminal line until a spinner replaces it or StepFinished commits it.
func (s *StepProgress) StepStarted(id distribution.StepID) {
	pos, ok := s.index[id]
	if !ok {
		return
	}
	s.writeSink("step: started " + s.label(pos))
	lineReg.register(s)
	line := MutedStyle.Render(s.label(pos))
	lineReg.paint(s, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		_, _ = fmt.Fprint(s.w, "\r\x1b[2K"+line)
	})
}

// StepFinished rewrites r's step line in place with its duration and a
// ✔/✖/skip glyph, commits it to scrollback, and mirrors the record to the log
// sink. Tolerates a missing StepStarted (skipped/already-done steps).
func (s *StepProgress) StepFinished(r *distribution.StepResult) {
	pos, ok := s.index[r.StepID]
	if !ok {
		return
	}
	s.writeSink(s.plainStatus(r, pos))
	final := s.finalLine(r, pos)
	// withLine erases the active owner's line (a leftover spinner or this
	// checklist's own in-progress line) before the commit, so the final line
	// never lands on a half-painted frame.
	lineReg.withLine(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		_, _ = fmt.Fprint(s.w, "\r\x1b[2K"+final+"\n")
	})
	lineReg.deregister(s)
}

// DeployFinished releases any line still owned at the end of a phase run; the
// total duration is unused — each step already reported its own.
func (s *StepProgress) DeployFinished(time.Duration) { lineReg.release(s) }

// clearLine implements lineOwner. The caller holds the line lock.
func (s *StepProgress) clearLine() {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = fmt.Fprint(s.w, "\r\x1b[2K")
}

func (s *StepProgress) counter(n int) string {
	width := len(strconv.Itoa(s.total))
	return fmt.Sprintf("[%*d/%d]", width, n, s.total)
}

func (s *StepProgress) label(pos stepPos) string {
	return fmt.Sprintf("%s %s · %s", s.counter(pos.n), pos.meta.Name, pos.meta.Phase)
}

func (s *StepProgress) finalLine(r *distribution.StepResult, pos stepPos) string {
	dur := r.Duration.Truncate(time.Millisecond).String()
	switch {
	case r.Skipped:
		return MutedStyle.Render(fmt.Sprintf("%s %s %s", s.label(pos), IconSkip, "skipped"))
	case r.Success:
		return fmt.Sprintf("%s  %s (%s)",
			TextStyle.Render(s.label(pos)), SuccessStyle.Render(IconSuccess), dur)
	default:
		return fmt.Sprintf("%s  %s (%s)",
			TextStyle.Render(s.label(pos)), ErrorStyle.Render(IconError), dur)
	}
}

func (s *StepProgress) plainStatus(r *distribution.StepResult, pos stepPos) string {
	switch {
	case r.Skipped:
		return "step: skip " + s.label(pos)
	case r.Success:
		return fmt.Sprintf("step: ok %s (%s)", s.label(pos), r.Duration.Truncate(time.Millisecond))
	default:
		return fmt.Sprintf("step: fail %s (%s)", s.label(pos), r.Duration.Truncate(time.Millisecond))
	}
}

// writeSink writes line straight to logSink, bypassing logutil.RedactHandler.
// Callers must only ever pass static step identifiers and numbers (step ID,
// counter, phase, duration) here — never config- or credential-derived
// strings — since nothing downstream scrubs this trail.
func (s *StepProgress) writeSink(line string) {
	if s.logSink == nil {
		return
	}
	_, _ = fmt.Fprintln(s.logSink, line)
}
