package deployexec

import (
	"errors"
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/logview"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

type deployScenario struct {
	name  string
	id    wizard.StepID
	build func() *State
	seed  func(m *wizard.Model, st *State)
	// hooks overrides goldenHooks for scenarios needing their own ring.
	hooks func() Hooks
}

// goldenHooks is the seeded feed every deploy golden renders against: no engine,
// a fixed log ring, and a fixed sink path so the pane, the narrow tail, and the
// full-log pointers are deterministic.
func goldenHooks() Hooks {
	return Hooks{
		Logs:    seededRing(24),
		LogPath: "okd-install/okdctl.log",
		Finish: &FinishHooks{
			ManageNodes: func() ([]wizard.WizardStep, wizard.FlowChrome, error) {
				return []wizard.WizardStep{finishTestStep{}}, wizard.FlowChrome{}, nil
			},
			OpenConsole: func() tea.Cmd { return func() tea.Msg { return nil } },
		},
	}
}

// wrappedRing seeds goldenHooks' ring plus one long error line, so the
// full-screen frames pin space-only wrapping instead of …-clipping.
func wrappedRing() Hooks {
	h := goldenHooks()
	r := h.Logs.(*logview.Ring)
	r.Append(logview.Line{
		At:    logBase.Add(24 * 7 * time.Second),
		Level: "ERROR",
		Text:  "deploy infrastructure failed: proxmox task UPID:pve:0000ABCD:00512F30:66F2A1C4:qmclone:9001:root@pam: refused the clone request because local-lvm is out of space on node pve-02",
	})
	return h
}

// errGoldenFailure is the engine failure every failure golden renders.
var errGoldenFailure = errors.New(
	"deploy infrastructure failed: proxmox task UPID:pve:0000A1: vm 9001 already exists")

// errorEarlyRing seeds goldenHooks' fixture with an ERROR early in the ring,
// so the jump keys and the minimap lane both have an off-screen target.
func errorEarlyRing() Hooks {
	r := logview.NewRing(logview.DefaultCap)
	for i := range 24 {
		line := logview.Line{
			At:    logBase.Add(time.Duration(i*7) * time.Second),
			Level: "INFO",
			Text:  fmt.Sprintf("deploy step started step=step-%02d phase=setup", i),
		}
		if i == 4 {
			line.Level = "ERROR"
			line.Text = "etcd member 0 refused the join request"
		}
		r.Append(line)
	}
	return Hooks{Logs: r, LogPath: "okd-install/okdctl.log"}
}

// typeFilter opens the filter input on the active step and types pattern into
// it, one keystroke at a time, the way an operator would.
func typeFilter(m *wizard.Model, pattern string) {
	m.Update(tea.KeyPressMsg{Code: logview.KeyFilter, Text: "/"})
	for _, r := range pattern {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

// seedMidRun drives the stream step to a fixed mid-install frame: prep
// collapsed with its total, ignition expanded around a running row, infra and
// later phases still pending.
func seedMidRun(m *wizard.Model, _ *State) {
	s, ok := m.CurrentStep().(*StreamStep)
	if !ok {
		return
	}
	base := time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)
	cur := base
	s.now = func() time.Time { return cur }
	s.started = base
	s.buildRows()

	for _, m := range s.st.Plan[:2] {
		s.applyEvent(&Event{StepID: m.ID})
		cur = cur.Add(20 * time.Second)
		s.applyEvent(&Event{StepID: m.ID, Done: true, Took: 20 * time.Second})
	}
	s.applyEvent(&Event{StepID: s.st.Plan[2].ID})
	cur = cur.Add(35 * time.Second)
}

// seedStalledRun drives the run into wait-bootstrap and then two minutes
// of silence — past the step's 90s stall threshold.
func seedStalledRun(m *wizard.Model, _ *State) {
	s, ok := m.CurrentStep().(*StreamStep)
	if !ok {
		return
	}
	base := time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)
	cur := base
	s.now = func() time.Time { return cur }
	s.started = base
	s.buildRows()

	for _, meta := range s.st.Plan[:5] {
		s.applyEvent(&Event{StepID: meta.ID})
		cur = cur.Add(20 * time.Second)
		s.applyEvent(&Event{StepID: meta.ID, Done: true, Took: 20 * time.Second})
	}
	s.applyEvent(&Event{StepID: s.st.Plan[5].ID})
	cur = cur.Add(2 * time.Minute)
}

// seedFailedRun drives the run into the ignition phase and fails it there,
// then advances to the done screen — the path that hands the incident report
// its frozen checklist, which a State built by hand has no way to produce.
func seedFailedRun(m *wizard.Model, st *State) {
	s, ok := m.CurrentStep().(*StreamStep)
	if !ok {
		return
	}
	base := time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)
	cur := base
	s.now = func() time.Time { return cur }
	s.started = base
	s.buildRows()

	for _, meta := range s.st.Plan[:2] {
		s.applyEvent(&Event{StepID: meta.ID})
		cur = cur.Add(20 * time.Second)
		s.applyEvent(&Event{StepID: meta.ID, Done: true, Took: 20 * time.Second})
	}
	s.applyEvent(&Event{StepID: s.st.Plan[2].ID})
	cur = cur.Add(35 * time.Second)
	st.Elapsed = cur.Sub(base)
	s.Update(streamEventMsg{ev: Event{Final: true, Err: errGoldenFailure}})
	m.Update(wizard.JumpToStepMsg{StepID: StepIDDone})
}

// seedFullRun settles every step and delivers the final event, leaving the
// step inside its settle window.
func seedFullRun(m *wizard.Model, _ *State) {
	s, ok := m.CurrentStep().(*StreamStep)
	if !ok {
		return
	}
	base := time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)
	cur := base
	s.now = func() time.Time { return cur }
	s.started = base
	s.buildRows()

	for _, meta := range s.st.Plan {
		s.applyEvent(&Event{StepID: meta.ID})
		cur = cur.Add(20 * time.Second)
		s.applyEvent(&Event{StepID: meta.ID, Done: true, Took: 20 * time.Second})
	}
	s.Update(streamEventMsg{ev: Event{Final: true}})
}

func deployScenarios() []deployScenario {
	return []deployScenario{
		{
			name:  "stream",
			id:    StepIDStream,
			build: streamState,
			seed:  seedMidRun,
		},
		{
			// A pinned mid-animation frame: the running row went quiet 35s
			// ago, so its glyph pulses on the slow frame cadence — a pure
			// function of the shared clock's counter, never wall time.
			name:  "stream_frame16",
			id:    StepIDStream,
			build: streamState,
			seed: func(m *wizard.Model, st *State) {
				seedMidRun(m, st)
				m.Update(wizard.FrameMsg{Frame: 16})
			},
		},
		{
			// A run with persisted history: the bar's segments follow the
			// weight table, the ETA is speaking, and pending phases carry
			// their schedule.
			name: "stream_history",
			id:   StepIDStream,
			build: func() *State {
				st := streamState()
				st.History = seededHistory()
				return st
			},
			seed: seedMidRun,
		},
		{
			// The stall detector: two minutes of silence during the
			// bootstrap wait freezes the running row to the amber marker
			// with its honest last-output reading.
			name: "stream_stalled",
			id:   StepIDStream,
			build: func() *State {
				st := streamState()
				st.History = seededHistory()
				return st
			},
			seed: seedStalledRun,
		},
		{
			// A pinned settle frame: the final event landed, the bar is
			// two frames into its critically-damped ease from the 96% cap
			// to 100% — frame-counter-driven, never wall time.
			name:  "stream_settle_frame2",
			id:    StepIDStream,
			build: streamState,
			seed: func(m *wizard.Model, st *State) {
				seedFullRun(m, st)
				m.Update(wizard.FrameMsg{Frame: 2})
			},
		},
		{
			// The graceful-cancel amber line, shown under the headline while
			// the current step finishes.
			name:  "stream_cancel",
			id:    StepIDStream,
			build: streamState,
			seed: func(m *wizard.Model, st *State) {
				seedMidRun(m, st)
				if s, ok := m.CurrentStep().(*StreamStep); ok {
					s.InterceptQuit()
				}
			},
		},
		{
			// The log window frozen where `l` locked it, header marked so a
			// still tail never reads as a stalled install.
			name:  "stream_locked",
			id:    StepIDStream,
			build: streamState,
			seed: func(m *wizard.Model, st *State) {
				seedMidRun(m, st)
				m.Update(tea.KeyPressMsg{Code: logview.KeyLock, Text: "l"})
				m.Update(wizard.FocusChangedMsg{})
			},
		},
		{
			// `f` swapped the log full-screen: the checklist steps aside and the
			// log takes the whole frame, its sink path in the header.
			name:  "stream_full",
			id:    StepIDStream,
			build: streamState,
			seed: func(m *wizard.Model, st *State) {
				seedMidRun(m, st)
				m.Update(tea.KeyPressMsg{Code: logview.KeyFull, Text: "f"})
				m.Update(wizard.LayoutChangedMsg{})
			},
		},
		{
			// A long error line wraps whole in full-screen mode instead of
			// …-clipping, its stamp column held by indented continuation rows.
			name:  "stream_wrapped",
			id:    StepIDStream,
			build: streamState,
			hooks: wrappedRing,
			seed: func(m *wizard.Model, st *State) {
				seedMidRun(m, st)
				m.Update(tea.KeyPressMsg{Code: logview.KeyFull, Text: "f"})
				m.Update(wizard.LayoutChangedMsg{})
			},
		},
		{
			// pgup in full-screen mode engages the lock and pages back through
			// the ring; the header names the window's honest coordinates.
			name:  "stream_paged",
			id:    StepIDStream,
			build: streamState,
			seed: func(m *wizard.Model, st *State) {
				seedMidRun(m, st)
				m.Update(tea.KeyPressMsg{Code: logview.KeyFull, Text: "f"})
				m.Update(wizard.LayoutChangedMsg{})
				m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
			},
		},
		{
			// The filter input mid-typing: the chip leads with the "/" that
			// opened it and counts matches from the first keystroke.
			name:  "stream_filtering",
			id:    StepIDStream,
			build: streamState,
			seed: func(m *wizard.Model, st *State) {
				seedMidRun(m, st)
				typeFilter(m, "step-2")
			},
		},
		{
			// The committed filter: only matching rows, and the chip names
			// the pattern it is selecting on.
			name:  "stream_filtered",
			id:    StepIDStream,
			build: streamState,
			seed: func(m *wizard.Model, st *State) {
				seedMidRun(m, st)
				typeFilter(m, "step-2")
				m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			},
		},
		{
			// A pattern that matches nothing: the rows go, the chip stays,
			// and the region says why it is empty.
			name:  "stream_filter_empty",
			id:    StepIDStream,
			build: streamState,
			seed: func(m *wizard.Model, st *State) {
				seedMidRun(m, st)
				typeFilter(m, "ceph")
			},
		},
		{
			// N walked the window back to the run's one error, which the
			// minimap lane had been marking all along.
			name:  "stream_jumped",
			id:    StepIDStream,
			build: streamState,
			hooks: errorEarlyRing,
			seed: func(m *wizard.Model, st *State) {
				seedMidRun(m, st)
				m.Update(tea.KeyPressMsg{Code: logview.KeyPrevMatch, Text: "N"})
				m.Update(wizard.FocusChangedMsg{})
			},
		},
		{
			name:  "done",
			id:    StepIDDone,
			build: doneState,
		},
		{
			// The failure frame: render.ErrorCard inside the wizard chrome,
			// shown once State.Result carries the engine error.
			name: "done_failure",
			id:   StepIDDone,
			build: func() *State {
				st := doneState()
				st.Result = errGoldenFailure
				return st
			},
		},
		{
			// The full incident report, reached the way a real failure reaches
			// it: the checklist frozen where the run stopped above the card,
			// the run's identity, the next moves, then the evidence.
			name:  "done_incident",
			id:    StepIDStream,
			build: streamState,
			seed:  seedFailedRun,
		},
		{
			// A single ctrl+c during the stream: the same report, but every
			// place it names the outcome reads interrupted, not failed — no
			// red ✗ on the step the operator stopped, no raw context.Canceled
			// text standing in for an explanation.
			name:  "done_incident_cancelled",
			id:    StepIDStream,
			build: streamState,
			seed:  seedCancelledRun,
		},
	}
}

var deployGoldenSizes = []struct {
	w, h int
}{
	{80, 24},
	{100, 30},
	{120, 40},
	{180, 48},
}

func TestGolden_DeploySteps(t *testing.T) {
	for _, sz := range deployGoldenSizes {
		for _, sc := range deployScenarios() {
			t.Run(fmt.Sprintf("%s_%dx%d", sc.name, sz.w, sz.h), func(t *testing.T) {
				// NewFlowModel seeds its initial size from the process's real
				// terminal (pipes under go test, or CI) rather than sz; pin it
				// so the golden is independent of that.
				tui.SetTerminalWidth(sz.w)
				t.Cleanup(func() { tui.SetTerminalWidth(0) })

				st := sc.build()
				hooks := goldenHooks
				if sc.hooks != nil {
					hooks = sc.hooks
				}
				m := wizard.NewFlowModel(NewSteps(st, hooks()), st.Cfg, Chrome())

				_ = tuitest.RenderAt(t, m, sz.w, sz.h)
				m.Update(wizard.JumpToStepMsg{StepID: sc.id})
				if sc.seed != nil {
					sc.seed(m, st)
				}

				frame := tuitest.RenderAt(t, m, sz.w, sz.h)
				tuitest.Golden(t, fmt.Sprintf("%s_%dx%d", sc.name, sz.w, sz.h), frame)
				tuitest.AssertFits(t, frame, sz.w, sz.h)
			})
		}
	}
}
