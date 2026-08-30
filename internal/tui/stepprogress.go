package tui

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/qxtaiba/okdctl/internal/distribution"
)

// StepMeta identifies one planned deploy step: ID, display name, and phase
// label. deploy builds the list from phases that will actually run, so a
// resume seeds a shorter checklist total.
type StepMeta struct {
	ID    distribution.StepID
	Name  string
	Phase string
}

// StepProgress renders a live, rewriting deploy step checklist to stderr,
// mirroring each step to the log sink (implements distribution.
// MetricsRecorder). It's a line owner: StepStarted takes ownership, a
// spawned spinner may replace it mid-step, and StepFinished always commits
// the final line regardless of current owner.
type StepProgress struct {
	w       io.Writer
	logSink io.Writer
	total   int
	index   map[distribution.StepID]stepPos
}

type stepPos struct {
	n    int
	meta StepMeta
}

// NewStepProgress builds a checklist recorder rendering to stderr; logSink
// (may be nil) mirrors per-step records so okdctl.log keeps the trail the
// TTY checklist replaces. Wire it only when ProgressBarsEnabled.
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

// SuppressStepLog reports that the orchestrator's per-step Info lines are
// redundant while this recorder renders the checklist, so they demote to
// Debug on the TTY; the trail still reaches okdctl.log via writeSink.
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
		_, _ = fmt.Fprint(s.w, "\r\x1b[2K"+line)
	})
}

// StepFinished rewrites r's step line with its duration and a ✔/✖/skip
// glyph, commits it to scrollback, and mirrors to the log sink; tolerates a
// missing StepStarted.
func (s *StepProgress) StepFinished(r *distribution.StepResult) {
	pos, ok := s.index[r.StepID]
	if !ok {
		return
	}
	s.writeSink(s.plainStatus(r, pos))
	final := s.finalLine(r, pos)
	// Clears any leftover spinner/checklist line first so the commit never
	// lands on a half-painted frame.
	lineReg.withLine(func() {
		_, _ = fmt.Fprint(s.w, "\r\x1b[2K"+final+"\n")
	})
	lineReg.deregister(s)
}

// DeployFinished releases any line still owned at the end of a phase run;
// total duration is unused.
func (s *StepProgress) DeployFinished(time.Duration) { lineReg.release(s) }

// clearLine implements lineOwner; the caller holds the line lock.
func (s *StepProgress) clearLine() {
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

// writeSink writes line straight to logSink, bypassing RedactHandler;
// callers must pass only static step identifiers/numbers, never config- or
// credential-derived strings, since nothing downstream scrubs this trail.
func (s *StepProgress) writeSink(line string) {
	if s.logSink == nil {
		return
	}
	_, _ = fmt.Fprintln(s.logSink, line)
}
