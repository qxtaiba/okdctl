package tui

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

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

// formatElapsed's precision ladder must never let the rendered duration
// exceed durationCol, even for hour-scale Terraform/bootstrap steps — the
// boundaries below straddle each rung (just under/over 10m and 1h).
func TestFormatElapsed_NeverExceedsDurationCol(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"just under 10m keeps 100ms precision", 9*time.Minute + 59*time.Second + 940*time.Millisecond, "9m59.9s"},
		{"just over 10m drops to seconds", 10*time.Minute + 500*time.Millisecond, "10m0s"},
		{"just under 1h keeps seconds", 59*time.Minute + 59*time.Second + 900*time.Millisecond, "59m59s"},
		{"just over 1h drops to minutes", time.Hour + time.Second, "1h0m"},
		{"under 10h keeps hours+minutes", 9*time.Hour + 59*time.Minute + 59*time.Second, "9h59m"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatElapsed(tc.d)
			if got != tc.want {
				t.Errorf("formatElapsed(%s) = %q, want %q", tc.d, got, tc.want)
			}
			if w := lipgloss.Width(got); w > durationCol {
				t.Errorf("formatElapsed(%s) = %q width %d exceeds durationCol %d", tc.d, got, w, durationCol)
			}
		})
	}
}

// Two labels of very different lengths must still land their glyph in the
// same visual column, since finalLine pads every label to labelWidth.
func TestStepProgress_GlyphColumnIsFixed(t *testing.T) {
	plan := []StepMeta{
		{ID: "short", Name: "n", Phase: "setup"},
		{ID: "long", Name: "a considerably longer step name", Phase: "postinstall"},
	}
	sp := newStepProgress(plan, nil, nil)

	short := sp.finalLine(&distribution.StepResult{StepID: "short", Success: true, Duration: time.Second}, sp.index["short"])
	long := sp.finalLine(&distribution.StepResult{StepID: "long", Success: true, Duration: time.Second}, sp.index["long"])

	shortCol := lipgloss.Width(strings.SplitN(short, IconSuccess, 2)[0])
	longCol := lipgloss.Width(strings.SplitN(long, IconSuccess, 2)[0])
	if shortCol != longCol {
		t.Errorf("glyph column drifted: short label col=%d, long label col=%d", shortCol, longCol)
	}
}

// A skipped line's glyph and status field must land in the same columns as
// a success/failure line, and durationCol="skipped" (7 runes) keeps the
// overall line width identical across every outcome.
func TestStepProgress_SkippedSharesColumns(t *testing.T) {
	sp := newStepProgress(threeStepPlan(), nil, nil)

	skip := sp.finalLine(&distribution.StepResult{StepID: "gen-config", Skipped: true, Success: true}, sp.index["gen-config"])
	ok := sp.finalLine(&distribution.StepResult{StepID: "create-vms", Success: true, Duration: 12 * time.Second}, sp.index["create-vms"])

	skipCol := lipgloss.Width(strings.SplitN(skip, IconSkip, 2)[0])
	okCol := lipgloss.Width(strings.SplitN(ok, IconSuccess, 2)[0])
	if skipCol != okCol {
		t.Errorf("skip glyph column %d != success glyph column %d", skipCol, okCol)
	}
	if w1, w2 := lipgloss.Width(skip), lipgloss.Width(ok); w1 != w2 {
		t.Errorf("skip line width %d != success line width %d", w1, w2)
	}
}

// Prefix reflects whichever step is between StepStarted and StepFinished,
// padded like finalLine's label, and empty between steps.
func TestStepProgress_PrefixDuringStep(t *testing.T) {
	var tty, sink bytes.Buffer
	sp := newStepProgress(threeStepPlan(), &tty, &sink)
	t.Cleanup(func() { lineReg.release(sp) })

	if got := sp.Prefix(); got != "" {
		t.Errorf("Prefix before any step started = %q, want empty", got)
	}

	sp.StepStarted("create-vms")
	want := fmt.Sprintf("%-*s", sp.labelWidth, sp.label(sp.index["create-vms"]))
	if got := sp.Prefix(); got != want {
		t.Errorf("Prefix during step = %q, want %q", got, want)
	}

	sp.StepFinished(&distribution.StepResult{StepID: "create-vms", Success: true, Duration: time.Second})
	if got := sp.Prefix(); got != "" {
		t.Errorf("Prefix after step finished = %q, want empty", got)
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
