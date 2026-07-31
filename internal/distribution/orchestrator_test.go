package distribution_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/errtypes"
)

func buildOrchestrator(defs []distribution.StepDef) *distribution.Orchestrator {
	return distribution.NewOrchestrator(distribution.BuildSteps(defs)...)
}

func TestOrchestratorRun_FatalStepStopsRun(t *testing.T) {
	t.Parallel()
	var ran []string
	defs := []distribution.StepDef{
		{
			ID: "a", Name: "a", ReRunSafe: distribution.ReRunSafeYes,
			Exec: func(_ context.Context) error { ran = append(ran, "a"); return nil },
		},
		{
			ID: "b", Name: "b", ReRunSafe: distribution.ReRunSafeYes,
			Exec: func(_ context.Context) error { ran = append(ran, "b"); return errors.New("boom") },
		},
		{
			ID: "c", Name: "c", ReRunSafe: distribution.ReRunSafeYes,
			Exec: func(_ context.Context) error { ran = append(ran, "c"); return nil },
		},
	}
	o := buildOrchestrator(defs)
	if err := o.Run(context.Background()); err == nil {
		t.Fatal("expected error from fatal step")
	}
	if want := []string{"a", "b"}; !slices.Equal(ran, want) {
		t.Fatalf("ran = %v, want %v (step c must not run after fatal failure)", ran, want)
	}
	results := o.Results()
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[1].Success {
		t.Error("step b result.Success = true, want false")
	}
}

func TestOrchestratorRun_NonFatalStepContinues(t *testing.T) {
	t.Parallel()
	var ran []string
	defs := []distribution.StepDef{
		{
			ID: "a", Name: "a", ReRunSafe: distribution.ReRunSafeYes, NonFatal: true,
			Exec: func(_ context.Context) error { ran = append(ran, "a"); return errors.New("warn") },
		},
		{
			ID: "b", Name: "b", ReRunSafe: distribution.ReRunSafeYes,
			Exec: func(_ context.Context) error { ran = append(ran, "b"); return nil },
		},
	}
	o := buildOrchestrator(defs)
	if err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run() = %v, want nil (non-fatal step must not stop run)", err)
	}
	if want := []string{"a", "b"}; !slices.Equal(ran, want) {
		t.Fatalf("ran = %v, want %v", ran, want)
	}
}

func TestOrchestratorRun_SkipWhen(t *testing.T) {
	t.Parallel()
	var ran bool
	defs := []distribution.StepDef{
		{
			ID: "a", Name: "a", ReRunSafe: distribution.ReRunSafeYes,
			SkipWhen: func() bool { return true }, SkipReason: "not needed",
			Exec: func(_ context.Context) error { ran = true; return nil },
		},
	}
	o := buildOrchestrator(defs)
	if err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	if ran {
		t.Error("step executed despite SkipWhen returning true")
	}
	results := o.Results()
	if len(results) != 1 || !results[0].Skipped || results[0].SkipReason != "not needed" {
		t.Fatalf("results = %+v, want single skipped result with reason %q", results, "not needed")
	}
}

// TestOrchestratorRun_SkipReasonFuncResolvesFiredCause pins that a
// SkipReasonFunc set alongside a static SkipReason wins, and that the
// recorded reason is the one resolved after SkipWhen fired.
func TestOrchestratorRun_SkipReasonFuncResolvesFiredCause(t *testing.T) {
	t.Parallel()
	reason := "unresolved"
	defs := []distribution.StepDef{
		{
			ID: "a", Name: "a", ReRunSafe: distribution.ReRunSafeYes,
			SkipWhen:       func() bool { reason = "cause that fired"; return true },
			SkipReason:     "static or-list",
			SkipReasonFunc: func() string { return reason },
			Exec:           func(_ context.Context) error { return nil },
		},
	}
	o := buildOrchestrator(defs)
	if err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	results := o.Results()
	if len(results) != 1 || !results[0].Skipped {
		t.Fatalf("results = %+v, want single skipped result", results)
	}
	if results[0].SkipReason != "cause that fired" {
		t.Errorf("SkipReason = %q, want the resolved cause, not the static string", results[0].SkipReason)
	}
}

func TestOrchestratorRun_AlreadyDoneSkipsExecute(t *testing.T) {
	t.Parallel()
	var ran bool
	defs := []distribution.StepDef{
		{
			ID: "a", Name: "a", ReRunSafe: distribution.ReRunSafeNo,
			AlreadyDone: func(_ context.Context) (bool, error) { return true, nil },
			Exec:        func(_ context.Context) error { ran = true; return nil },
		},
	}
	o := buildOrchestrator(defs)
	if err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	if ran {
		t.Error("step executed despite AlreadyDone returning true")
	}
	results := o.Results()
	if len(results) != 1 || !results[0].Skipped || results[0].SkipReason != "already done" {
		t.Fatalf("results = %+v, want single skipped result with reason %q", results, "already done")
	}
}

func TestOrchestratorRun_AlreadyDoneErrorProceedsToExecute(t *testing.T) {
	t.Parallel()
	var ran bool
	defs := []distribution.StepDef{
		{
			ID: "a", Name: "a", ReRunSafe: distribution.ReRunSafeNo,
			AlreadyDone: func(_ context.Context) (bool, error) { return false, errors.New("probe failed") },
			Exec:        func(_ context.Context) error { ran = true; return nil },
		},
	}
	o := buildOrchestrator(defs)
	if err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	if !ran {
		t.Error("step did not execute after AlreadyDone returned an error")
	}
}

func TestOrchestratorRun_CallbacksFireInOrder(t *testing.T) {
	t.Parallel()
	var events []string
	defs := []distribution.StepDef{
		{
			ID: "a", Name: "a", ReRunSafe: distribution.ReRunSafeYes,
			OnStart: func() { events = append(events, "start") },
			Exec: func(_ context.Context) error {
				events = append(events, "exec")
				return nil
			},
		},
	}
	o := buildOrchestrator(defs)
	if err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	if want := []string{"start", "exec"}; !slices.Equal(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
}

func TestOrchestratorRun_OnErrorFiresWithClassifiedError(t *testing.T) {
	t.Parallel()
	var gotErr error
	defs := []distribution.StepDef{
		{
			ID: "a", Name: "a", ReRunSafe: distribution.ReRunSafeYes,
			Exec:    func(_ context.Context) error { return errors.New("boom") },
			OnError: func(err error) { gotErr = err },
		},
	}
	o := buildOrchestrator(defs)
	_ = o.Run(context.Background())
	var ce *errtypes.ClusterError
	if !errors.As(gotErr, &ce) {
		t.Fatalf("OnError err = %T, want *errtypes.ClusterError (classifyStepErr backstop)", gotErr)
	}
}

func TestOrchestratorRun_ClassifiesBareErrorAsClusterError(t *testing.T) {
	t.Parallel()
	defs := []distribution.StepDef{
		{
			ID: "a", Name: "a", ReRunSafe: distribution.ReRunSafeYes,
			Exec: func(_ context.Context) error { return errors.New("boom") },
		},
	}
	o := buildOrchestrator(defs)
	err := o.Run(context.Background())
	var ce *errtypes.ClusterError
	if !errors.As(err, &ce) {
		t.Fatalf("Run() err = %T, want *errtypes.ClusterError", err)
	}
	// errtypes Error() surfaces only Msg, so the root cause must be carried
	// in Msg or every sink prints a bare "step failed".
	if !strings.Contains(ce.Error(), "boom") {
		t.Fatalf("classified error %q does not surface the root cause", ce.Error())
	}
}

func TestOrchestratorRun_PreservesTypedErrtypesError(t *testing.T) {
	t.Parallel()
	want := &errtypes.ConfigError{Msg: "bad config"}
	defs := []distribution.StepDef{
		{
			ID: "a", Name: "a", ReRunSafe: distribution.ReRunSafeYes,
			Exec: func(_ context.Context) error { return want },
		},
	}
	o := buildOrchestrator(defs)
	err := o.Run(context.Background())
	if !errors.Is(err, want) {
		t.Fatalf("Run() err = %v, want the original *errtypes.ConfigError preserved unwrapped", err)
	}
}

func TestOrchestratorRun_PreservesContextCancellation(t *testing.T) {
	t.Parallel()
	defs := []distribution.StepDef{
		{
			ID: "a", Name: "a", ReRunSafe: distribution.ReRunSafeYes,
			Exec: func(_ context.Context) error { return context.Canceled },
		},
	}
	o := buildOrchestrator(defs)
	err := o.Run(context.Background())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() err = %v, want context.Canceled preserved unwrapped", err)
	}
}

func TestOrchestratorRun_CtxDoneBeforeStepStopsRun(t *testing.T) {
	t.Parallel()
	var ran bool
	defs := []distribution.StepDef{
		{
			ID: "a", Name: "a", ReRunSafe: distribution.ReRunSafeYes,
			Exec: func(_ context.Context) error { ran = true; return nil },
		},
	}
	o := buildOrchestrator(defs)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := o.Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() err = %v, want context.Canceled", err)
	}
	if ran {
		t.Error("step executed despite pre-cancelled context")
	}
}
