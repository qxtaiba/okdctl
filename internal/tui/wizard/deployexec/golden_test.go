package deployexec

import (
	"errors"
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
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
	return Hooks{Logs: seededRing(24), LogPath: "okd-install/okdctl.log"}
}

// wrappedRing seeds goldenHooks' ring plus one long error line, so the
// full-screen frames pin space-only wrapping instead of …-clipping.
func wrappedRing() Hooks {
	h := goldenHooks()
	r := h.Logs.(*LogRing)
	r.append(LogLine{
		At:    logBase.Add(24 * 7 * time.Second),
		Level: "ERROR",
		Text:  "deploy infrastructure failed: proxmox task UPID:pve:0000ABCD:00512F30:66F2A1C4:qmclone:9001:root@pam: refused the clone request because local-lvm is out of space on node pve-02",
	})
	return h
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

func deployScenarios() []deployScenario {
	return []deployScenario{
		{
			name:  "stream",
			id:    StepIDStream,
			build: streamState,
			seed:  seedMidRun,
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
				m.Update(tea.KeyPressMsg{Code: keyLogLock, Text: "l"})
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
				m.Update(tea.KeyPressMsg{Code: keyLogFull, Text: "f"})
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
				m.Update(tea.KeyPressMsg{Code: keyLogFull, Text: "f"})
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
				m.Update(tea.KeyPressMsg{Code: keyLogFull, Text: "f"})
				m.Update(wizard.LayoutChangedMsg{})
				m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
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
				st.Result = errors.New("deploy infrastructure failed: proxmox task UPID:pve:0000A1: vm 9001 already exists")
				return st
			},
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
