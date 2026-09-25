package deployexec

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/install"
)

func TestRecorderTranslatesStepTransitions(t *testing.T) {
	events := make(chan Event, 4)
	rec := NewRecorder(context.Background(), events)

	rec.StepStarted(install.StepDeployInfra)
	rec.StepFinished(&distribution.StepResult{StepID: install.StepDeployInfra, Success: true, Duration: 90 * time.Second})
	rec.StepFinished(&distribution.StepResult{StepID: install.StepWaitBootstrap, Skipped: true})
	boom := errors.New("apply failed")
	rec.StepFinished(&distribution.StepResult{StepID: install.StepStartWorkers, Error: boom})

	want := []Event{
		{StepID: install.StepDeployInfra},
		{StepID: install.StepDeployInfra, Done: true, Took: 90 * time.Second},
		{StepID: install.StepWaitBootstrap, Done: true, Skipped: true},
		{StepID: install.StepStartWorkers, Done: true, Err: boom},
	}
	for i, w := range want {
		got := <-events
		if got != w {
			t.Errorf("event %d = %+v, want %+v", i, got, w)
		}
	}
}

// TestRecorderSendAbortsOnCancelledContext proves a quit can never strand the
// engine's goroutine on a channel nobody drains.
func TestRecorderSendAbortsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	// Unbuffered: the send can only complete through the ctx branch.
	rec := NewRecorder(ctx, make(chan Event))
	cancel()

	done := make(chan struct{})
	go func() {
		rec.StepStarted(install.StepDeployInfra)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("StepStarted blocked on a cancelled context")
	}
}

// TestRecorderDeployFinishedIsInert pins the reason the run's end comes from the
// Execute hook's return: the orchestrator calls DeployFinished once per phase.
func TestRecorderDeployFinishedIsInert(t *testing.T) {
	events := make(chan Event, 1)
	NewRecorder(context.Background(), events).DeployFinished(time.Minute)
	if len(events) != 0 {
		t.Errorf("DeployFinished emitted %d events, want 0", len(events))
	}
}
