package deployexec

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

type deployScenario struct {
	name  string
	id    wizard.StepID
	build func() *State
	seed  func(m *wizard.Model, st *State)
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
				m := wizard.NewFlowModel(NewSteps(st, Hooks{}), st.Cfg, Chrome())

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
