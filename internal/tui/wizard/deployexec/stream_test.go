package deployexec

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/install"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/postinstall"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/setup"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// streamPlan is the checklist fixture the stream tests drive: real step ids
// spread across four of the five phases, so collapsed, expanded, and pending
// groups render side by side.
func streamPlan() []tui.StepMeta {
	return []tui.StepMeta{
		{ID: setup.StepInstallPackages, Name: setup.StepNames[setup.StepInstallPackages], Phase: "setup"},
		{ID: setup.StepDownloadTools, Name: setup.StepNames[setup.StepDownloadTools], Phase: "setup"},
		{ID: setup.StepGenerateIgnition, Name: setup.StepNames[setup.StepGenerateIgnition], Phase: "setup"},
		{ID: setup.StepBuildISOs, Name: setup.StepNames[setup.StepBuildISOs], Phase: "setup"},
		{ID: install.StepDeployInfra, Name: install.StepNames[install.StepDeployInfra], Phase: "install"},
		{ID: install.StepWaitBootstrap, Name: install.StepNames[install.StepWaitBootstrap], Phase: "install"},
		{ID: postinstall.StepVerifyHealth, Name: postinstall.StepNames[postinstall.StepVerifyHealth], Phase: "postinstall"},
	}
}

func streamState() *State {
	cfg := config.DefaultConfig()
	cfg.Cluster.Name = "homelab"
	cfg.Cluster.Domain = "lab.example.com"
	return &State{Cfg: cfg, Plan: streamPlan(), RunID: "run-01"}
}

// newSeededStreamStep constructs a StreamStep with rows built and a fixed clock
// installed, without starting the engine goroutine — the package-internal
// seeding path golden and unit tests drive the checklist deterministically with.
func newSeededStreamStep(st *State, clock *time.Time) *StreamStep {
	s := NewStreamStep(st, Hooks{})
	s.now = func() time.Time { return *clock }
	s.started = *clock
	s.buildRows()
	return s
}

// pump replays bubbletea's cmd loop to StepCompleteMsg, dropping spinner ticks
// so it terminates.
func pump(t *testing.T, s *StreamStep, first tea.Cmd) tea.Msg {
	t.Helper()
	queue := []tea.Cmd{first}
	for range 200 {
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
	t.Fatal("deploy never completed")
	return nil
}

func TestStreamGroupsPlanIntoPhasesInExecutionOrder(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s := newSeededStreamStep(streamState(), &cur)

	got := make([]Phase, len(s.phases))
	for i := range s.phases {
		got[i] = s.phases[i].name
	}
	want := []Phase{PhasePrep, PhaseIgnition, PhaseInfra, PhaseInstall, PhaseVerify}
	if len(got) != len(want) {
		t.Fatalf("phases = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("phases = %v, want %v", got, want)
		}
	}
}

// TestStreamDropsPhasesAResumeSkips proves a resumed run never shows a phase it
// will not execute sitting pending forever.
func TestStreamDropsPhasesAResumeSkips(t *testing.T) {
	st := streamState()
	st.Plan = []tui.StepMeta{
		{ID: install.StepWaitBootstrap, Name: "wait for bootstrap", Phase: "install"},
		{ID: postinstall.StepVerifyHealth, Name: "verify cluster health", Phase: "postinstall"},
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s := newSeededStreamStep(st, &cur)

	if len(s.phases) != 2 {
		t.Fatalf("phases = %d, want only the two a postinstall-onwards resume runs", len(s.phases))
	}
	out := tuitest.StripANSI(s.View(100, 40))
	if strings.Contains(out, string(PhasePrep)) {
		t.Errorf("a resumed run must not list the prep phase:\n%s", out)
	}
}

func TestStreamViewCollapsesFinishedAndExpandsCurrent(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s := newSeededStreamStep(streamState(), &cur)

	s.applyEvent(&Event{StepID: setup.StepInstallPackages})
	cur = base.Add(60 * time.Second)
	s.applyEvent(&Event{StepID: setup.StepGenerateIgnition})

	out := tuitest.StripANSI(s.View(100, 40))

	var prepLine, runningRow string
	sawPendingVerify := false
	for _, l := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(l)
		switch {
		case strings.Contains(l, string(PhasePrep)):
			prepLine = l
		case strings.Contains(l, "generate ignition"):
			runningRow = trimmed
		case trimmed == tui.IconPending+" "+string(PhaseVerify):
			sawPendingVerify = true
		}
	}

	if !strings.HasPrefix(strings.TrimSpace(prepLine), tui.IconSuccess+" "+string(PhasePrep)) {
		t.Errorf("prep must render collapsed with a success icon, got %q", prepLine)
	}
	if !strings.HasSuffix(strings.TrimRight(prepLine, " "), "1m0s") {
		t.Errorf("prep line = %q, want it to end with 1m0s", prepLine)
	}
	if runningRow == "" {
		t.Fatalf("the running ignition row must render:\n%s", out)
	}
	if strings.HasPrefix(runningRow, tui.IconSuccess) || strings.HasPrefix(runningRow, tui.IconPending) {
		t.Errorf("the running row must use the spinner glyph, got %q", runningRow)
	}
	if !sawPendingVerify {
		t.Errorf("verify must render collapsed pending:\n%s", out)
	}
}

// TestStreamLogKeysInertWithoutLogs pins bug 27: with no Logs hook the
// unadvertised f/l keys must do nothing rather than blank the whole body
// behind an empty full-screen log.
func TestStreamLogKeysInertWithoutLogs(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s := newSeededStreamStep(streamState(), &cur)

	before := tuitest.StripANSI(s.View(100, 40))
	step, _ := s.Update(tea.KeyPressMsg{Code: keyLogFull, Text: "f"})
	s = step.(*StreamStep)

	if got := tuitest.StripANSI(s.View(100, 40)); got != before {
		t.Fatalf("f with a nil Logs hook changed the body:\nbefore:\n%s\nafter:\n%s", before, got)
	}
}

// TestStreamCancelDropsCtrlCFootnote pins bug 28: once a cancel is
// requested the next ctrl+c force-quits, so the footnote must stop
// promising a safe cancel.
func TestStreamCancelDropsCtrlCFootnote(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s := newSeededStreamStep(streamState(), &cur)

	if out := tuitest.StripANSI(s.View(100, 40)); !strings.Contains(out, "ctrl+c cancels") {
		t.Fatalf("pre-cancel view must advertise the safe cancel:\n%s", out)
	}

	if !s.InterceptQuit() {
		t.Fatal("first ctrl+c must be intercepted as a graceful cancel")
	}
	out := tuitest.StripANSI(s.View(100, 40))
	if !strings.Contains(out, "cancel requested") {
		t.Fatalf("cancel notice missing:\n%s", out)
	}
	if strings.Contains(out, "ctrl+c cancels") {
		t.Fatalf("stale ctrl+c footnote survives its own cancel:\n%s", out)
	}
}

func TestStreamRowDurationsTruncateToSeconds(t *testing.T) {
	if got := fmtDur(90*time.Second + 700*time.Millisecond); got != "1m30s" {
		t.Errorf("fmtDur = %q, want 1m30s", got)
	}

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s := newSeededStreamStep(streamState(), &cur)

	s.applyEvent(&Event{StepID: setup.StepInstallPackages})
	s.applyEvent(&Event{StepID: setup.StepInstallPackages, Done: true, Took: 90*time.Second + 700*time.Millisecond})

	out := tuitest.StripANSI(s.View(100, 40))
	if !strings.Contains(out, "1m30s") {
		t.Errorf("view must show the truncated duration:\n%s", out)
	}
	if strings.Contains(out, "1m30.7s") {
		t.Errorf("view must truncate sub-second precision:\n%s", out)
	}
}

func TestStreamSkippedRowReadsSkipped(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s := newSeededStreamStep(streamState(), &cur)

	s.applyEvent(&Event{StepID: setup.StepInstallPackages})
	s.applyEvent(&Event{StepID: setup.StepInstallPackages, Done: true, Skipped: true})

	out := tuitest.StripANSI(s.View(100, 40))
	var row string
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, setup.StepNames[setup.StepInstallPackages]) {
			row = strings.TrimSpace(l)
		}
	}
	if !strings.HasPrefix(row, tui.IconSkip) {
		t.Errorf("skipped row = %q, want it to start with the skip glyph", row)
	}
	if !strings.HasSuffix(strings.TrimRight(row, " "), "skipped") {
		t.Errorf("skipped row = %q, want it to end with skipped", row)
	}
}

func TestStreamHeadlineCountsSettledStepsAndRightAlignsElapsed(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s := newSeededStreamStep(streamState(), &cur)

	s.applyEvent(&Event{StepID: setup.StepInstallPackages})
	s.applyEvent(&Event{StepID: setup.StepInstallPackages, Done: true, Took: time.Second})
	cur = base.Add(6*time.Minute + 12*time.Second)

	const width = 100
	line := strings.Split(s.View(width, 40), "\n")[0]
	if got := lipgloss.Width(line); got != width-4 {
		t.Errorf("headline width = %d, want %d", got, width-4)
	}
	plain := tuitest.StripANSI(line)
	if !strings.Contains(plain, "deploying homelab  1 / 7") {
		t.Errorf("headline = %q, want the settled-step counter", plain)
	}
	if !strings.HasSuffix(strings.TrimRight(plain, " "), "elapsed 6m12s") {
		t.Errorf("headline = %q, want it to end with the elapsed time", plain)
	}
}

func TestStreamFocusedSpanTracksRunningRow(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s := newSeededStreamStep(streamState(), &cur)

	s.applyEvent(&Event{StepID: setup.StepDownloadTools})
	out := s.View(100, 40)
	span, ok := s.FocusedSpan()
	if !ok {
		t.Fatal("FocusedSpan must report ok once rows exist")
	}
	if span.Start != span.End {
		t.Fatalf("running-row span must be a single line, got %+v", span)
	}
	lines := strings.Split(out, "\n")
	if !strings.Contains(lines[span.Start], setup.StepNames[setup.StepDownloadTools]) {
		t.Errorf("span line = %q, want the running row", lines[span.Start])
	}

	s.finished = true
	out = s.View(100, 40)
	span, _ = s.FocusedSpan()
	if want := len(strings.Split(out, "\n")) - 1; span.Start != want {
		t.Errorf("finished span = %+v, want last line %d", span, want)
	}
}

func TestStreamFailedRowShowsDuration(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s := newSeededStreamStep(streamState(), &cur)

	s.applyEvent(&Event{StepID: install.StepDeployInfra})
	cur = base.Add(2 * time.Minute)
	_, _ = s.Update(streamEventMsg{ev: Event{Final: true, Err: errors.New("terraform apply failed")}})

	out := tuitest.StripANSI(s.View(100, 40))
	var failedRow string
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, install.StepNames[install.StepDeployInfra]) {
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

// TestStreamUnplannedStepDegradesVisibly proves an engine step the plan never
// listed still shows up, instead of vanishing from the checklist.
func TestStreamUnplannedStepDegradesVisibly(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s := newSeededStreamStep(streamState(), &cur)

	s.applyEvent(&Event{StepID: setup.StepInstallPackages})
	s.applyEvent(&Event{StepID: "a-step-no-plan-listed"})

	out := tuitest.StripANSI(s.View(100, 40))
	if !strings.Contains(out, "a-step-no-plan-listed") {
		t.Errorf("an unplanned step must render under the running row:\n%s", out)
	}
}

func TestStreamRunsToCompletionAndRecordsOutcome(t *testing.T) {
	st := streamState()
	executed := false
	s := NewStreamStep(st, Hooks{Execute: func(_ *State, ch chan<- Event) error {
		executed = true
		for _, m := range st.Plan {
			ch <- Event{StepID: m.ID}
			ch <- Event{StepID: m.ID, Done: true, Took: time.Second}
		}
		return nil
	}})

	if _, ok := pump(t, s, s.Init()).(wizard.StepCompleteMsg); !ok {
		t.Fatal("a finished run must advance to the done screen")
	}
	if !executed {
		t.Error("Execute hook never ran")
	}
	if !st.Executed || st.Result != nil {
		t.Errorf("state = executed %v result %v, want executed with no error", st.Executed, st.Result)
	}
}

func TestStreamFailurePropagatesToState(t *testing.T) {
	st := streamState()
	boom := errors.New("terraform apply failed")
	s := NewStreamStep(st, Hooks{Execute: func(*State, chan<- Event) error { return boom }})

	if _, ok := pump(t, s, s.Init()).(wizard.StepCompleteMsg); !ok {
		t.Fatal("a failure must still advance to the done screen")
	}
	if !errors.Is(st.Result, boom) {
		t.Errorf("Result = %v, want the engine error", st.Result)
	}
}

func TestStreamQuitGuardCancelsThenForces(t *testing.T) {
	cancelled := false
	s := NewStreamStep(streamState(), Hooks{
		CancelDeploy: func() { cancelled = true },
		Execute:      func(*State, chan<- Event) error { return nil },
	})
	if !s.InterceptQuit() {
		t.Fatal("first ctrl+c must be intercepted")
	}
	if !cancelled {
		t.Fatal("first ctrl+c must invoke CancelDeploy")
	}
	if s.InterceptQuit() {
		t.Fatal("second ctrl+c must pass through (force quit)")
	}
}

func TestStreamAndDoneStepsAreForwardOnly(t *testing.T) {
	st := streamState()
	if !NewStreamStep(st, Hooks{}).InterceptBack() {
		t.Error("stream step must intercept esc — navigating away orphans the event pump")
	}
	if !NewDoneStep(st, Hooks{}).InterceptBack() {
		t.Error("done step must intercept esc — going back re-enters a finished run")
	}
}

func TestJustifyClampsOversizedRight(t *testing.T) {
	got := justify("short", "a right side longer than the available width", 10)
	if w := lipgloss.Width(got); w > 10 {
		t.Errorf("justify() = %q, width %d, want <= 10", got, w)
	}
}

// TestStreamNarrowTailFillsTheBodySlack pins the tail budget: on a narrow
// frame with rows to spare, the log tail grows past its six-row floor to
// fill the body instead of idling blank rows under the clauses line.
func TestStreamNarrowTailFillsTheBodySlack(t *testing.T) {
	tui.SetTerminalWidth(120)
	t.Cleanup(func() { tui.SetTerminalWidth(0) })

	st := streamState()
	m := wizard.NewFlowModel(NewSteps(st, goldenHooks()), st.Cfg, Chrome())
	_ = tuitest.RenderAt(t, m, 120, 40)
	m.Update(wizard.JumpToStepMsg{StepID: StepIDStream})
	seedMidRun(m, st)

	frame := tuitest.StripANSI(tuitest.RenderAt(t, m, 120, 40))
	if got := strings.Count(frame, "09:0"); got <= narrowTailRows {
		t.Fatalf("tail shows %d log rows at 120x40, want more than the %d-row floor", got, narrowTailRows)
	}
	tuitest.AssertFits(t, frame, 120, 40)
}

// TestStreamWindowTitleCarriesProgress pins the live window title during an
// install: percent settled plus the running phase, so a backgrounded
// terminal tab reports the run's state.
func TestStreamWindowTitleCarriesProgress(t *testing.T) {
	tui.SetTerminalWidth(100)
	t.Cleanup(func() { tui.SetTerminalWidth(0) })

	st := streamState()
	m := wizard.NewFlowModel(NewSteps(st, goldenHooks()), st.Cfg, Chrome())
	_ = tuitest.RenderAt(t, m, 100, 30)
	m.Update(wizard.JumpToStepMsg{StepID: StepIDStream})
	seedMidRun(m, st)

	if got, want := m.View().WindowTitle, "okdctl · deploying 28% · ignition"; got != want {
		t.Fatalf("WindowTitle = %q, want %q", got, want)
	}
}
