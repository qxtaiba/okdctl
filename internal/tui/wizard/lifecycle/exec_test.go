package lifecycle

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// pump replays bubbletea's cmd loop to StepCompleteMsg, dropping spinner ticks so it terminates.
func pump(t *testing.T, s *ExecStep, first tea.Cmd) tea.Msg {
	t.Helper()
	queue := []tea.Cmd{first}
	for range 50 {
		if len(queue) == 0 {
			t.Fatal("command queue drained before completion")
		}
		cmd := queue[0]
		queue = queue[1:]
		if cmd == nil {
			continue
		}
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			queue = append(queue, batch...)
			continue
		}
		if _, ok := msg.(wizard.StepCompleteMsg); ok {
			return msg
		}
		_, next := s.Update(msg)
		queue = append(queue, next)
	}
	t.Fatal("execution never completed")
	return nil
}

func execState() *State {
	st := doneState()
	st.Scope = node.ResizeScope{Role: nodetypes.RoleMaster}
	return st
}

// threeMasterState builds a resize-masters plan with three nodes, so tests
// can exercise the checklist's collapse/expand/pending states side by side;
// the config's cluster name matches the plan's, so trails never show a
// default-config name over homelab bodies.
func threeMasterState() *State {
	cfg := config.DefaultConfig()
	cfg.Cluster.Name = "homelab"
	return &State{
		Cfg: cfg, Op: node.OpResize,
		Scope: node.ResizeScope{Role: nodetypes.RoleMaster},
		Plan: &node.OpPlan{
			Op: node.OpResize, Cluster: "homelab",
			Nodes: []node.PlanNode{
				{Name: "homelab-master0", Role: nodetypes.RoleMaster},
				{Name: "homelab-master1", Role: nodetypes.RoleMaster},
				{Name: "homelab-master2", Role: nodetypes.RoleMaster},
			},
		},
		Proceed: true,
	}
}

// newSeededExecStep constructs an ExecStep against st with rows built and a
// fixed clock installed, without starting the Runner goroutine — the
// package-internal seeding path golden and unit tests use to drive the
// checklist deterministically.
func newSeededExecStep(st *State, clock *time.Time) *ExecStep {
	s := NewExecStep(st, Hooks{})
	s.now = func() time.Time { return *clock }
	s.started = *clock
	s.buildRows()
	return s
}

// TestRowDurDistinguishesImpliedFromMeasured pins bug 29: a row whose
// completion was inferred renders "—", a real sub-second measurement
// renders "<1s", and only genuine measurements render as durations.
func TestRowDurDistinguishesImpliedFromMeasured(t *testing.T) {
	cases := []struct {
		name string
		row  execRow
		want string
	}{
		{"implied", execRow{status: rowDone, implied: true}, "—"},
		{"sub-second", execRow{status: rowDone, took: 400 * time.Millisecond}, "<1s"},
		{"measured", execRow{status: rowDone, took: 3 * time.Second}, "3s"},
	}
	for _, tc := range cases {
		if got := rowDur(&tc.row); got != tc.want {
			t.Errorf("%s: rowDur() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestPromoteRowDoneMarksBackfilledRowsImplied(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	pending := execRow{status: rowPending}
	promoteRowDone(&pending, now)
	if !pending.implied {
		t.Error("a promoted pending row must be marked implied")
	}

	measured := execRow{status: rowRunning, start: now.Add(-2 * time.Second)}
	promoteRowDone(&measured, now)
	if measured.implied {
		t.Error("a promoted running row carries a real elapsed, not an implied one")
	}
}

// TestNodeIndexForNeverCollidesWorker1WithWorker10 pins bug 26: an event
// whose description mentions homelab-worker10 must resolve to worker10,
// not to worker1 via a bare substring match.
func TestNodeIndexForNeverCollidesWorker1WithWorker10(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	st := threeMasterState()
	st.Plan.Nodes = []node.PlanNode{
		{Name: "homelab-worker1", Role: nodetypes.RoleWorker},
		{Name: "homelab-worker10", Role: nodetypes.RoleWorker},
	}
	s := newSeededExecStep(st, &cur)

	got := s.nodeIndexFor(&ExecEvent{Desc: "power-cycle homelab-worker10"})
	if got != 1 {
		t.Fatalf("nodeIndexFor(worker10 desc) = %d, want 1", got)
	}
	got = s.nodeIndexFor(&ExecEvent{Node: "homelab-worker10"})
	if got != 1 {
		t.Fatalf("nodeIndexFor(worker10 node) = %d, want 1", got)
	}
	got = s.nodeIndexFor(&ExecEvent{Desc: "cordon + drain homelab-worker1"})
	if got != 0 {
		t.Fatalf("nodeIndexFor(worker1 desc) = %d, want 0", got)
	}
}

func TestExecViewCollapsesFinishedAndExpandsCurrent(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s := newSeededExecStep(threeMasterState(), &cur)

	s.applyEvent(&ExecEvent{Node: "homelab-master0", Step: node.StepTFApply})
	cur = base.Add(60 * time.Second)
	s.applyEvent(&ExecEvent{Node: "homelab-master1", Step: node.StepTFApply})

	out := tuitest.StripANSI(s.View(100, 40))
	lines := strings.Split(out, "\n")

	var m0Line, runningRow string
	sawM2Pending := false
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		switch {
		case strings.Contains(l, "homelab-master0"):
			m0Line = l
		case strings.Contains(l, "terraform apply (in-place update)"):
			runningRow = trimmed
		case trimmed == tui.IconPending+" homelab-master2":
			sawM2Pending = true
		}
	}

	if !strings.HasPrefix(strings.TrimSpace(m0Line), tui.IconSuccess+" homelab-master0") {
		t.Errorf("m0 must render collapsed with a success icon, got %q", m0Line)
	}
	if !strings.HasSuffix(strings.TrimRight(m0Line, " "), "1m0s") {
		t.Errorf("m0 line = %q, want it to end with 1m0s", m0Line)
	}
	if runningRow == "" {
		t.Fatalf("m1's running row must render:\n%s", out)
	}
	if strings.HasPrefix(runningRow, tui.IconSuccess) || strings.HasPrefix(runningRow, tui.IconPending) || strings.HasPrefix(runningRow, tui.IconError) {
		t.Errorf("m1's running row must use the spinner glyph, got %q", runningRow)
	}
	if !sawM2Pending {
		t.Errorf("m2 must render collapsed pending:\n%s", out)
	}
}

func TestExecFocusedSpanTracksRunningRow(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s := newSeededExecStep(threeMasterState(), &cur)

	t.Run("running row", func(t *testing.T) {
		s.applyEvent(&ExecEvent{Node: "homelab-master0", Step: node.StepTFApply})
		out := s.View(100, 40)
		span, ok := s.FocusedSpan()
		if !ok {
			t.Fatal("FocusedSpan must report ok once rows exist")
		}
		lines := strings.Split(out, "\n")
		if span.Start != span.End {
			t.Fatalf("running-row span must be a single line, got %+v", span)
		}
		if !strings.Contains(lines[span.Start], "terraform apply (in-place update)") {
			t.Errorf("span line = %q, want the running row", lines[span.Start])
		}
	})

	t.Run("next node", func(t *testing.T) {
		s.applyEvent(&ExecEvent{Node: "homelab-master1", Step: node.StepCordon})
		out := s.View(100, 40)
		span, ok := s.FocusedSpan()
		if !ok {
			t.Fatal("FocusedSpan must report ok on the second node")
		}
		lines := strings.Split(out, "\n")
		if !strings.Contains(lines[span.Start], "cordon + drain") {
			t.Errorf("span line after advancing = %q, want the new running row", lines[span.Start])
		}
	})

	t.Run("finished", func(t *testing.T) {
		s.finished = true
		out := s.View(100, 40)
		span, ok := s.FocusedSpan()
		if !ok {
			t.Fatal("FocusedSpan must report ok when finished")
		}
		if want := len(strings.Split(out, "\n")) - 1; span.Start != want {
			t.Errorf("finished span = %+v, want last line %d", span, want)
		}
	})
}

// TestExecFootnoteDropsCancelHintOnceFinished guards E-L8(f): once the op
// has finished there is nothing left to cancel, so repeating "ctrl+c
// cancels after the current gate" reads as stale advice on an otherwise-done
// screen — the footnote must still name the marker file, just not the
// cancel clause.
func TestExecFootnoteDropsCancelHintOnceFinished(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s := newSeededExecStep(threeMasterState(), &cur)

	running := s.View(100, 40)
	if !strings.Contains(running, "ctrl+c cancels after the current gate") {
		t.Fatalf("running view must still show the cancel hint:\n%s", running)
	}
	if !strings.Contains(running, "marker okd-install/") {
		t.Fatalf("running view lost the marker path:\n%s", running)
	}

	s.finished = true
	finished := s.View(100, 40)
	if strings.Contains(finished, "ctrl+c cancels after the current gate") {
		t.Fatalf("finished view still shows the stale cancel hint:\n%s", finished)
	}
	if !strings.Contains(finished, "marker okd-install/") {
		t.Fatalf("finished view lost the marker path:\n%s", finished)
	}
}

// TestExecShortHelpMatchesStreamStepCancelLabel guards NEW(T8): the
// lifecycle exec screen's cancel label must read identically to the deploy
// stream screen's, so the same gesture reads as one system on both
// full-screen exec surfaces.
func TestExecShortHelpMatchesStreamStepCancelLabel(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s := newSeededExecStep(threeMasterState(), &cur)

	bindings := s.ShortHelp()
	if len(bindings) != 1 || bindings[0].Help != "cancel (twice to force-quit)" {
		t.Fatalf("ShortHelp() = %+v, want a single \"cancel (twice to force-quit)\" binding", bindings)
	}
}

func TestExecRowDurationsTruncateToSeconds(t *testing.T) {
	if got := fmtDur(90*time.Second + 700*time.Millisecond); got != "1m30s" {
		t.Errorf("fmtDur = %q, want 1m30s", got)
	}

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s := newSeededExecStep(threeMasterState(), &cur)

	s.applyEvent(&ExecEvent{Node: "homelab-master0", Desc: "cordoning and draining homelab-master0"})
	s.applyEvent(&ExecEvent{
		Node: "homelab-master0", Desc: "cordoning and draining homelab-master0",
		Done: true, Took: 90*time.Second + 700*time.Millisecond,
	})

	out := tuitest.StripANSI(s.View(100, 40))
	if !strings.Contains(out, "1m30s") {
		t.Errorf("view must show the truncated duration:\n%s", out)
	}
	if strings.Contains(out, "1m30.7s") || strings.Contains(out, ".7s") {
		t.Errorf("view must truncate sub-second precision:\n%s", out)
	}
}

func TestExecHeadlineRightAlignsElapsed(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	elapsed := base.Add(6*time.Minute + 12*time.Second)
	s := newSeededExecStep(threeMasterState(), &elapsed)
	s.started = base

	const width = 100
	out := s.View(width, 40)
	line := strings.Split(out, "\n")[0]
	if got := lipgloss.Width(line); got != width-4 {
		t.Errorf("headline width = %d, want %d", got, width-4)
	}
	plain := tuitest.StripANSI(line)
	if !strings.HasSuffix(strings.TrimRight(plain, " "), "elapsed 6m12s") {
		t.Errorf("headline = %q, want it to end with the elapsed time", plain)
	}
}

func TestExecExtraShownOnlyUnderRunningRow(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s := newSeededExecStep(threeMasterState(), &cur)

	s.applyEvent(&ExecEvent{Node: "homelab-master0", Step: node.StepTFApply})
	s.applyEvent(&ExecEvent{Node: "homelab-master0", Desc: "polling proxmox task status"})

	out := tuitest.StripANSI(s.View(100, 40))
	if !strings.Contains(out, "polling proxmox task status") {
		t.Fatalf("extra must render under the running row:\n%s", out)
	}

	s.applyEvent(&ExecEvent{
		Node: "homelab-master0", Desc: "applying terraform change to m.master[0]",
		Done: true, Took: time.Second,
	})
	out = tuitest.StripANSI(s.View(100, 40))
	if strings.Contains(out, "polling proxmox task status") {
		t.Errorf("extra must not render once no row is running:\n%s", out)
	}

	s.applyEvent(&ExecEvent{Node: "homelab-master0", Step: node.StepPowerCycle})
	out = tuitest.StripANSI(s.View(100, 40))
	if strings.Contains(out, "polling proxmox task status") {
		t.Errorf("stale extra must not leak onto the next running row:\n%s", out)
	}
}

func TestExecFailedRowShowsDuration(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s := newSeededExecStep(threeMasterState(), &cur)

	s.applyEvent(&ExecEvent{Node: "homelab-master0", Step: node.StepTFApply})
	cur = base.Add(2 * time.Minute)
	_, _ = s.Update(execEventMsg{ev: ExecEvent{Final: true, Err: errors.New("terraform apply failed")}})

	out := tuitest.StripANSI(s.View(100, 40))
	var failedRow string
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "terraform apply (in-place update)") {
			failedRow = strings.TrimSpace(l)
		}
	}
	if !strings.HasPrefix(failedRow, tui.IconError) {
		t.Fatalf("failed row = %q, want it to start with the error icon", failedRow)
	}
	if !strings.HasSuffix(strings.TrimRight(failedRow, " "), "2m0s") {
		t.Errorf("failed row = %q, want it to end with 2m0s", failedRow)
	}
}

func TestJustifyClampsOversizedRight(t *testing.T) {
	got := justify("short", "a right side longer than the available width", 10)
	if w := lipgloss.Width(got); w > 10 {
		t.Errorf("justify() = %q, width %d, want <= 10", got, w)
	}
}

func TestExecStepRunsToCompletion(t *testing.T) {
	st := execState()
	executed := false
	s := NewExecStep(st, Hooks{Execute: func(_ *State, ch chan<- ExecEvent) error {
		executed = true
		ch <- ExecEvent{Node: "homelab-master0", Step: node.StepTFApply}
		ch <- ExecEvent{Desc: "waiting for etcd health (post-homelab-master0)", Done: true}
		return nil
	}})
	msg := pump(t, s, s.Init())
	if _, ok := msg.(wizard.StepCompleteMsg); !ok {
		t.Fatalf("final msg = %T, want StepCompleteMsg", msg)
	}
	if !executed {
		t.Error("Execute hook never ran")
	}
	if st.Result != nil {
		t.Errorf("Result = %v, want nil", st.Result)
	}
	out := s.View(90, 40)
	if !strings.Contains(out, "homelab-master0") {
		t.Errorf("view must render the node section:\n%s", out)
	}
}

func TestExecStepFailurePropagatesToState(t *testing.T) {
	st := execState()
	boom := errors.New("etcd health gate failed")
	s := NewExecStep(st, Hooks{Execute: func(*State, chan<- ExecEvent) error { return boom }})
	msg := pump(t, s, s.Init())
	if _, ok := msg.(wizard.StepCompleteMsg); !ok {
		t.Fatalf("failure must still advance to the done screen, got %T", msg)
	}
	if !errors.Is(st.Result, boom) {
		t.Errorf("Result = %v, want the execute error", st.Result)
	}
}

func TestExecStepQuitGuardCancelsThenForces(t *testing.T) {
	cancelled := false
	s := NewExecStep(execState(), Hooks{
		CancelOp: func() { cancelled = true },
		Execute:  func(*State, chan<- ExecEvent) error { return nil },
	})
	if !s.InterceptQuit() {
		t.Fatal("first ctrl+c must be intercepted")
	}
	if !cancelled {
		t.Fatal("first ctrl+c must invoke CancelOp")
	}
	if s.InterceptQuit() {
		t.Fatal("second ctrl+c must pass through (force quit)")
	}
}

func TestExecStepShouldShow(t *testing.T) {
	cfg := config.DefaultConfig()
	if NewExecStep(&State{Cfg: cfg, Proceed: false}, Hooks{}).ShouldShow(cfg) {
		t.Error("exec step must hide without consent")
	}
	if !NewExecStep(execState(), Hooks{}).ShouldShow(cfg) {
		t.Error("exec step must show after consent")
	}
}

func TestExecAndDoneStepsAreForwardOnly(t *testing.T) {
	st := execState()
	if !NewExecStep(st, Hooks{}).InterceptBack() {
		t.Error("exec step must intercept esc — navigating away orphans the event pump")
	}
	if !NewDoneStep(st).InterceptBack() {
		t.Error("done step must intercept esc — going back re-enters a finished run")
	}
}

// TestExecStylesFollowThemeFlip pins flip-safety for a CLI-launched flow:
// the step is constructed before the terminal's background reply lands, so
// its styles must resolve at render time, not freeze their dark values.
func TestExecStylesFollowThemeFlip(t *testing.T) {
	t.Cleanup(func() { tui.SetDarkBackground(true) })
	tui.SetDarkBackground(true)

	s := NewExecStep(threeMasterState(), Hooks{})
	base := time.Date(2026, 8, 30, 9, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return base }
	s.started = base
	s.buildRows()

	tui.SetDarkBackground(false)
	if out := s.View(100, 40); !strings.Contains(out, "15;23;42") {
		t.Errorf("post-flip headline misses the light Text tier (#0F172A):\n%q", out)
	}
}
