package tui

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/distribution"
)

func threeStepPlan() []StepMeta {
	return []StepMeta{
		{ID: "gen-config", Name: "generate config", Phase: "setup"},
		{ID: "create-vms", Name: "create vms", Phase: "install"},
		{ID: "verify", Name: "verify health", Phase: "postinstall"},
	}
}

// TestStepProgress_StartThenFinishRewritesLine drives one step through
// StepStarted → StepFinished and asserts the dim in-progress line is committed
// as a permanent line carrying the counter, name, phase, duration and ✔.
func TestStepProgress_StartThenFinishRewritesLine(t *testing.T) {
	var tty, sink bytes.Buffer
	sp := newStepProgress(threeStepPlan(), &tty, &sink)
	t.Cleanup(func() { lineReg.release(sp) })

	sp.StepStarted("create-vms")
	if !lineReg.hasOwner() {
		t.Fatal("StepStarted did not register the checklist as line owner")
	}
	sp.StepFinished(&distribution.StepResult{StepID: "create-vms", Success: true, Duration: 12 * time.Second})

	out := tty.String()
	for _, want := range []string{"[2/3]", "create vms", "install", IconSuccess, "12s"} {
		if !strings.Contains(out, want) {
			t.Errorf("tty output missing %q:\n%q", want, out)
		}
	}
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("finished line not committed to scrollback (no trailing newline):\n%q", out)
	}
	if lineReg.hasOwner() {
		t.Error("checklist still owns the line after StepFinished")
	}
	if !strings.Contains(sink.String(), "step: ok [2/3] create vms · install") {
		t.Errorf("log sink missing per-step record:\n%q", sink.String())
	}
}

// TestStepProgress_FinishWithoutStart covers skipped/already-done steps that
// emit only StepFinished: the renderer must still commit a final line and not
// panic on the missing start.
func TestStepProgress_FinishWithoutStart(t *testing.T) {
	var tty, sink bytes.Buffer
	sp := newStepProgress(threeStepPlan(), &tty, &sink)

	sp.StepFinished(&distribution.StepResult{StepID: "gen-config", Skipped: true, Success: true})

	out := tty.String()
	for _, want := range []string{"[1/3]", "generate config", "setup", IconSkip, "skipped"} {
		if !strings.Contains(out, want) {
			t.Errorf("tty output missing %q:\n%q", want, out)
		}
	}
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("skipped line not committed:\n%q", out)
	}
	if !strings.Contains(sink.String(), "step: skip [1/3] generate config · setup") {
		t.Errorf("log sink missing skip record:\n%q", sink.String())
	}
}

// TestStepProgress_FailureStyling asserts a failed step commits the ✖ glyph.
func TestStepProgress_FailureStyling(t *testing.T) {
	var tty, sink bytes.Buffer
	sp := newStepProgress(threeStepPlan(), &tty, &sink)

	sp.StepFinished(&distribution.StepResult{StepID: "verify", Success: false, Duration: 3 * time.Second})

	out := tty.String()
	if !strings.Contains(out, IconError) {
		t.Errorf("failed line missing %q:\n%q", IconError, out)
	}
	if !strings.Contains(sink.String(), "step: fail [3/3] verify health · postinstall") {
		t.Errorf("log sink missing fail record:\n%q", sink.String())
	}
}

// TestStepProgress_ResumeSubsetTotals proves the counter total reflects only
// the steps seeded (a resumed run's subset), not a full deploy, and that the
// counter is padded to the total's digit width.
func TestStepProgress_ResumeSubsetTotals(t *testing.T) {
	plan := []StepMeta{
		{ID: "verify", Name: "verify health", Phase: "postinstall"},
		{ID: "cleanup", Name: "cleanup bootstrap", Phase: "postinstall"},
	}
	var tty bytes.Buffer
	sp := newStepProgress(plan, &tty, nil)
	t.Cleanup(func() { lineReg.release(sp) })

	sp.StepStarted("cleanup")
	if got := tty.String(); !strings.Contains(got, "[2/2]") {
		t.Errorf("resume-subset counter wrong; want [2/2] in:\n%q", got)
	}
}

// TestStepProgress_CounterPadding locks the right-aligned counter width to the
// total's digit count, e.g. "[ 4/17]".
func TestStepProgress_CounterPadding(t *testing.T) {
	plan := make([]StepMeta, 17)
	for i := range plan {
		plan[i] = StepMeta{ID: distribution.StepID(string(rune('a' + i))), Name: "n", Phase: "install"}
	}
	sp := newStepProgress(plan, nil, nil)
	if got := sp.counter(4); got != "[ 4/17]" {
		t.Errorf("counter(4) = %q, want %q", got, "[ 4/17]")
	}
}

// TestStepProgress_UnknownStepIgnored: an id absent from the plan is a no-op,
// never a panic (guards against a phase emitting a step the plan omitted).
func TestStepProgress_UnknownStepIgnored(t *testing.T) {
	var tty bytes.Buffer
	sp := newStepProgress(threeStepPlan(), &tty, nil)
	sp.StepStarted("nonexistent")
	sp.StepFinished(&distribution.StepResult{StepID: "nonexistent", Success: true})
	if tty.Len() != 0 {
		t.Errorf("unknown step produced output:\n%q", tty.String())
	}
}

// TestStepProgress_ConcurrentInterleave drives the real runtime shape the
// sequential tests above never exercise: one orchestrator goroutine cycling
// StepStarted/StepFinished across several steps, a spinner taking the line
// as the second owner during each step's simulated work (exactly as
// tui.StartSpinner does inside a real step body), and N goroutines emitting
// tui.Info records concurrently throughout. -race is the actual assertion;
// the buffer checks below only confirm every committed checklist line
// survived the interleave exactly once and at column 0.
func TestStepProgress_ConcurrentInterleave(t *testing.T) {
	plan := []StepMeta{
		{ID: "gen-config", Name: "generate config", Phase: "setup"},
		{ID: "create-vms", Name: "create vms", Phase: "install"},
		{ID: "wait-bootstrap", Name: "wait for bootstrap", Phase: "install"},
		{ID: "verify", Name: "verify health", Phase: "postinstall"},
		{ID: "cleanup", Name: "cleanup bootstrap", Phase: "postinstall"},
	}

	var buf, sink bytes.Buffer
	configureBuf(t, &buf)
	sp := newStepProgress(plan, &buf, &sink)
	t.Cleanup(func() { lineReg.release(sp) })

	const goroutines, perG = 10, 60
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := range goroutines {
		go func(id int) {
			defer wg.Done()
			for range perG {
				Info("checklist interleave", LF("g", id))
			}
		}(g)
	}

	// Each step hands the line to a fresh spinner exactly as a real step
	// body would, then StepFinished commits while that spinner is still the
	// registered owner (the "leftover spinner" clear path in withLine).
	// Every spinner is stopped only after all log producers have joined
	// below, so lineReg's active flag never flips false while a concurrent
	// Info() call could still be mid-flight against it.
	ids := []distribution.StepID{"gen-config", "create-vms", "wait-bootstrap", "verify", "cleanup"}
	results := make([]*distribution.StepResult, len(ids))
	stopSpinners := make([]func(), 0, len(ids))
	for i, id := range ids {
		sp.StepStarted(id)
		stopSpinners = append(stopSpinners, startSpinner(context.Background(), "step working", &buf))
		// Real sleep on purpose — not a synctest candidate. In a bubble the
		// fake clock advances only when every goroutine is durably blocked,
		// so the busy Info() producers would defer this sleep until they
		// finish, collapsing the mid-step interleave window -race needs.
		time.Sleep(2 * time.Millisecond)
		results[i] = &distribution.StepResult{
			StepID:   id,
			Success:  id != "wait-bootstrap",
			Duration: time.Duration(i+1) * time.Second,
		}
		sp.StepFinished(results[i])
	}

	// Stop everything in order: join the log producers first, then release
	// every spinner, so no writer remains active when we inspect buf.
	wg.Wait()
	for _, stop := range stopSpinners {
		stop()
	}

	if lineReg.hasOwner() {
		t.Fatal("a line owner is still registered after teardown")
	}

	out := buf.String()
	for i, id := range ids {
		pos, ok := sp.index[id]
		if !ok {
			t.Fatalf("step %s missing from plan index", id)
		}
		want := "\r\x1b[2K" + sp.finalLine(results[i], pos) + "\n"
		if got := strings.Count(out, want); got != 1 {
			t.Errorf("step %s committed line count = %d, want exactly 1 (line must start at column 0 and appear once):\nwant=%q", id, got, want)
		}
	}
}
