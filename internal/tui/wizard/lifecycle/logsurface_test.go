package lifecycle

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/tui/logview"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// logBase is the fixed clock every seeded log fixture stamps from.
var logBase = time.Date(2026, 8, 30, 9, 0, 0, 0, time.UTC)

// errLifecycleFailed is the backend failure the evidence-tail tests render.
var errLifecycleFailed = errors.New("etcd health gate (post-master0) failed: quorum lost")

// seededRing fills a real ring with n deterministic lines, the fixture the
// production tee would hand the exec screen.
func seededRing(n int) *logview.Ring {
	r := logview.NewRing(logview.DefaultCap)
	for i := range n {
		r.Append(logview.Line{
			At:    logBase.Add(time.Duration(i*7) * time.Second),
			Level: "INFO",
			Text:  fmt.Sprintf("terraform apply step=step-%02d node=homelab-master0", i),
		})
	}
	return r
}

func TestLifecycleFlowStepCountMatchesNewSteps(t *testing.T) {
	if got := len(NewSteps(threeMasterState(), Hooks{})); got != flowStepCount {
		t.Errorf("NewSteps builds %d screens, flowStepCount says %d", got, flowStepCount)
	}
}

// TestExecNarrowFrameCarriesTheTailUnderTheChecklist pins proposal 3's core
// claim on the lifecycle flow: terraform applies and drains stream into the
// same instrument the deploy flow has, instead of running behind a blind
// spinner.
func TestExecNarrowFrameCarriesTheTailUnderTheChecklist(t *testing.T) {
	s := NewExecStep(threeMasterState(), Hooks{Logs: seededRing(20)})
	s.buildRows()
	s.SetTerminalSize(120, 40)
	s.SetSize(104, 30)

	body := tuitest.StripANSI(s.View(104, 1000))
	if !strings.Contains(body, "step-19") {
		t.Errorf("a narrow frame must carry the log tail:\n%s", body)
	}

	s.SetTerminalSize(180, 48)
	if strings.Contains(tuitest.StripANSI(s.View(100, 1000)), "step-19") {
		t.Error("a frame with a log pane must not repeat the tail in the body")
	}
	if !strings.Contains(tuitest.StripANSI(s.PaneContent(44, 8)), "step-19") {
		t.Error("the pane must carry the live log on the split tier")
	}
}

// TestExecLockAndFullKeys pins the shared key vocabulary reaching the
// lifecycle exec screen: l locks the window with honest coordinates, f swaps
// the log full-screen and asks the frame to re-measure.
func TestExecLockAndFullKeys(t *testing.T) {
	s := NewExecStep(threeMasterState(), Hooks{Logs: seededRing(10)})
	s.buildRows()
	s.SetSize(100, 20)
	s.SetTerminalSize(180, 48)

	if _, cmd := s.Update(tea.KeyPressMsg{Code: logview.KeyLock, Text: "l"}); cmd != nil {
		t.Error("locking the log must not emit a command")
	}
	if !s.log.Locked() {
		t.Fatal("l must lock the log")
	}
	if got := tuitest.StripANSI(s.PaneContent(44, 6)); !strings.Contains(got, "LOG · 6–10 of 10") {
		t.Errorf("a locked pane must name its window position:\n%s", got)
	}
	if !s.ConsumesPaging() {
		t.Error("a locked pane must claim the paging keys")
	}
	s.Update(tea.KeyPressMsg{Code: logview.KeyLock, Text: "l"})

	_, cmd := s.Update(tea.KeyPressMsg{Code: logview.KeyFull, Text: "f"})
	if cmd == nil {
		t.Fatal("f must ask the frame to re-measure")
	}
	if _, ok := cmd().(wizard.LayoutChangedMsg); !ok {
		t.Errorf("f emitted %T, want LayoutChangedMsg", cmd())
	}
	if !s.SuppressesSplit() {
		t.Error("a full-screen log must claim the whole frame")
	}
	body := tuitest.StripANSI(s.View(104, 1000))
	if strings.Contains(body, "etcd health gate") {
		t.Errorf("a full-screen log must replace the checklist:\n%s", body)
	}
	if !strings.Contains(body, "resizing masters") {
		t.Errorf("a full-screen log must keep the run's headline:\n%s", body)
	}
}

// TestExecShortHelpAdvertisesTheLogKeys is what puts them in the ? overlay:
// it renders exactly the step's own bindings.
func TestExecShortHelpAdvertisesTheLogKeys(t *testing.T) {
	s := NewExecStep(threeMasterState(), Hooks{Logs: seededRing(2)})

	keys := map[string]string{}
	for _, b := range s.ShortHelp() {
		keys[b.Key] = b.Help
	}
	if keys["l"] != "lock the log here" {
		t.Errorf("l help = %q, want the lock instruction", keys["l"])
	}
	if keys["f"] != "full-screen log" {
		t.Errorf("f help = %q, want the full-screen instruction", keys["f"])
	}
	if _, ok := keys[wizard.HelpCtrlC]; !ok {
		t.Error("the cancel binding must survive alongside the log keys")
	}

	bare := NewExecStep(threeMasterState(), Hooks{})
	for _, b := range bare.ShortHelp() {
		if b.Key == "l" || b.Key == "f" {
			t.Errorf("advertised %q with no log source", b.Key)
		}
	}
}

// TestLifecycleDoneFailureKeepsTheLastLogLinesOnScreen pins the failure
// card carrying its evidence: the ring's tail rides under the error card on
// a narrow frame, the pane carries it on the split tier, and the sink line
// names the file that keeps every byte.
func TestLifecycleDoneFailureKeepsTheLastLogLinesOnScreen(t *testing.T) {
	st := doneState()
	st.Result = errLifecycleFailed
	s := NewDoneStep(st, Hooks{Logs: seededRing(20), LogPath: "okd-install/okdctl.log"})
	s.SetTerminalSize(100, 30)

	out := tuitest.StripANSI(s.View(96, 1000))
	if !strings.Contains(out, "step-19") {
		t.Errorf("failure view must keep the last log lines on screen:\n%s", out)
	}
	if !strings.Contains(out, "full log okd-install/okdctl.log") {
		t.Errorf("failure view must name the sink that keeps every byte:\n%s", out)
	}

	s.SetTerminalSize(180, 48)
	if strings.Contains(tuitest.StripANSI(s.View(96, 1000)), "step-19") {
		t.Error("a frame with a log pane must not repeat the tail under the card")
	}
	if !strings.Contains(tuitest.StripANSI(s.PaneContent(44, 8)), "step-19") {
		t.Error("the pane must carry the tail on the completion screen too")
	}
}

// TestExecFinalSendAbortsOnACancelledRun pins bug 30's fix: a force-quit
// must never strand the runner goroutine holding the runlock on a feed
// nobody drains.
func TestExecFinalSendAbortsOnACancelledRun(t *testing.T) {
	gone := make(chan struct{})
	close(gone)

	s := NewExecStep(threeMasterState(), Hooks{Done: gone})
	for len(s.events) < cap(s.events) {
		s.events <- ExecEvent{}
	}

	returned := make(chan struct{})
	go func() {
		s.sendFinal(nil)
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("the final send blocked on a feed nobody drains")
	}
}

// TestExecFootnoteNamesTheResolvedSinkPath pins the footnote pointing at the
// path the run actually writes alongside the marker, never a guessed one.
func TestExecFootnoteNamesTheResolvedSinkPath(t *testing.T) {
	s := NewExecStep(threeMasterState(), Hooks{Logs: seededRing(4), LogPath: "/srv/lab/okdctl.log"})
	s.buildRows()
	s.SetTerminalSize(120, 40)

	body := tuitest.StripANSI(s.View(104, 1000))
	if !strings.Contains(body, "full log /srv/lab/okdctl.log") {
		t.Errorf("footnote must name the resolved sink path:\n%s", body)
	}
	if !strings.Contains(body, node.OpMarkerFileName) {
		t.Errorf("footnote must keep the marker pointer:\n%s", body)
	}

	bare := NewExecStep(threeMasterState(), Hooks{})
	bare.buildRows()
	bare.SetTerminalSize(120, 40)
	if body := tuitest.StripANSI(bare.View(104, 1000)); strings.Contains(body, "full log") {
		t.Errorf("with no sink open the footnote must not invent one:\n%s", body)
	}
}

// TestDemoHooksCarryALogStream keeps the demo honest with the real session:
// the scripted feed fills the same log surface the operator would see live.
func TestDemoHooksCarryALogStream(t *testing.T) {
	h := DemoHooks(0)
	if h.Logs == nil {
		t.Fatal("demo hooks must carry a log source for the pane")
	}
	if h.Done == nil {
		t.Fatal("demo hooks must carry the cancel channel for the final send")
	}

	st := threeMasterState()
	events := make(chan ExecEvent, 256)
	if err := h.Execute(st, events); err != nil {
		t.Fatalf("demo execute: %v", err)
	}
	lines, _ := h.Logs.Snapshot()
	if len(lines) == 0 {
		t.Error("the demo run must fill the log surface the way a real run does")
	}
}

// TestExecFinalDeliveredAfterGracefulCancel pins biased delivery: the first
// ctrl+c cancels the same context the final send selects on, and a uniform
// select would drop the event about half the time — stranding the exec
// screen on "cancel requested — finishing safely…". With buffer space free
// the final event must arrive every single time.
func TestExecFinalDeliveredAfterGracefulCancel(t *testing.T) {
	gone := make(chan struct{})
	close(gone)

	for range 200 {
		s := NewExecStep(threeMasterState(), Hooks{Done: gone})
		s.sendFinal(nil)
		select {
		case ev := <-s.events:
			if !ev.Final {
				t.Fatalf("delivered event = %+v, want the final one", ev)
			}
		default:
			t.Fatal("a graceful cancel dropped the final event despite buffer space")
		}
	}
}
