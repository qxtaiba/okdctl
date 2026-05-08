package distribution_test

import (
	"context"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/distribution"
)

func TestBuildSteps_PanicsOnReRunSafeNoWithoutAlreadyDone(t *testing.T) {
	t.Parallel()
	defs := []distribution.StepDef{
		{
			ID:        "test-step",
			Name:      "test step",
			ReRunSafe: distribution.ReRunSafeNo,
			Exec:      func(_ context.Context) error { return nil },
		},
	}
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected BuildSteps to panic for ReRunSafeNo without AlreadyDone")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("expected string panic value, got %T: %v", r, r)
		}
		const want = "ReRunSafeNo but has no AlreadyDone"
		if !strings.Contains(msg, want) {
			t.Fatalf("panic message %q does not contain %q", msg, want)
		}
	}()
	distribution.BuildSteps(defs)
}

func TestBuildSteps_AcceptsReRunSafeNoWithAlreadyDone(t *testing.T) {
	t.Parallel()
	defs := []distribution.StepDef{
		{
			ID:        "test-step",
			Name:      "test step",
			ReRunSafe: distribution.ReRunSafeNo,
			AlreadyDone: func(_ context.Context) (bool, error) {
				return false, nil
			},
			Exec: func(_ context.Context) error { return nil },
		},
	}
	steps := distribution.BuildSteps(defs)
	if len(steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(steps))
	}
}
