package tui

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/logutil"

	"github.com/qxtaiba/okdctl/internal/distribution"
)

func threeStepPlan() []StepMeta {
	return []StepMeta{
		{ID: "gen-config", Name: "generate config", Phase: "setup"},
		{ID: "create-vms", Name: "create vms", Phase: "install"},
		{ID: "verify", Name: "verify health", Phase: "postinstall"},
	}
}

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

func TestStepProgress_UnknownStepIgnored(t *testing.T) {
	var tty bytes.Buffer
	sp := newStepProgress(threeStepPlan(), &tty, nil)
	sp.StepStarted("nonexistent")
	sp.StepFinished(&distribution.StepResult{StepID: "nonexistent", Success: true})
	if tty.Len() != 0 {
		t.Errorf("unknown step produced output:\n%q", tty.String())
	}
}

// -race is the actual assertion here; the buffer checks only confirm
// ordering survived the interleave.
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
				logutil.Info("checklist interleave", logutil.LF("g", id))
			}
		}(g)
	}

	// Spinners stop only after all log producers join, keeping withLine's
	// "leftover spinner" clear path racing against Info() calls.
	ids := []distribution.StepID{"gen-config", "create-vms", "wait-bootstrap", "verify", "cleanup"}
	results := make([]*distribution.StepResult, len(ids))
	stopSpinners := make([]func(), 0, len(ids))
	for i, id := range ids {
		sp.StepStarted(id)
		stopSpinners = append(stopSpinners, startSpinner(context.Background(), "step working", &buf))
		// Real sleep on purpose — synctest's fake clock only advances when
		// every goroutine blocks, which would collapse the race window here.
		time.Sleep(2 * time.Millisecond)
		results[i] = &distribution.StepResult{
			StepID:   id,
			Success:  id != "wait-bootstrap",
			Duration: time.Duration(i+1) * time.Second,
		}
		sp.StepFinished(results[i])
	}

	// Join log producers before releasing spinners so no writer is active
	// when we inspect buf.
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
