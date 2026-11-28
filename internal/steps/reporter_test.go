package steps_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/steps"
)

type recordingReporter struct{ events []string }

func (r *recordingReporter) StepStarted(id steps.StepID, name string) {
	r.events = append(r.events, "started "+string(id)+" "+name)
}

func (r *recordingReporter) StepProgress(id steps.StepID, line string) {
	r.events = append(r.events, "progress "+string(id)+" "+line)
}

func (r *recordingReporter) StepFinished(res *steps.StepResult) {
	switch {
	case res.Skipped:
		r.events = append(r.events, "skipped "+string(res.StepID)+" "+res.SkipReason)
	case res.Error != nil:
		r.events = append(r.events, "failed "+string(res.StepID))
	default:
		r.events = append(r.events, "succeeded "+string(res.StepID))
	}
}

func TestOrchestratorRun_ReportsStepStream(t *testing.T) {
	t.Parallel()
	ok := func(context.Context) error { return nil }
	rep := &recordingReporter{}
	o := steps.NewOrchestrator(rep, steps.BuildSteps([]steps.StepDef{
		{ID: "a", Name: "build iso", Exec: ok},
		{ID: "b", Name: "b", SkipWhen: func() bool { return true }, SkipReason: "not needed", Exec: ok},
		{ID: "c", Name: "c", AlreadyDone: func(context.Context) (bool, error) { return false, errors.New("probe down") }, Exec: ok},
		{ID: "d", Name: "d", Exec: func(context.Context) error { return errors.New("boom") }},
		{ID: "e", Name: "e", Exec: ok},
	})...)
	if err := o.Run(context.Background()); err == nil {
		t.Fatal("Run() = nil, want the fatal error from d")
	}
	want := []string{
		"started a build iso", "succeeded a",
		"skipped b not needed",
		"progress c already-done check failed, proceeding: probe down", "started c c", "succeeded c",
		"started d d", "failed d",
	}
	if !slices.Equal(rep.events, want) {
		t.Errorf("events =\n%s\nwant\n%s", strings.Join(rep.events, "\n"), strings.Join(want, "\n"))
	}
}

func TestLogReporter_NarratesAndScrubs(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	rep := steps.LogReporter(slog.New(logutil.NewRedactHandler(slog.NewJSONHandler(&buf, nil))))
	rep.StepStarted("a", "build iso")
	rep.StepProgress("a", "Host m0 validation: password=hunter2 rejected")
	rep.StepFinished(&steps.StepResult{StepID: "a", Skipped: true, SkipReason: "already done"})
	rep.StepFinished(&steps.StepResult{StepID: "a", Error: errors.New("boom")})
	rep.StepFinished(&steps.StepResult{StepID: "a", Success: true})

	out := buf.String()
	for _, want := range []string{"step: started", "build iso", "step: progress", "Host m0 validation", "step: skipped", "already done", "step: failed", "boom", "step: succeeded"} {
		if !strings.Contains(out, want) {
			t.Errorf("log output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "hunter2") {
		t.Errorf("progress line leaked a credential past the sink:\n%s", out)
	}
}
