// Package distribution hosts the phase-step orchestration primitives
// (StepDef, BuildSteps, Orchestrator) shared by every distribution.
package distribution

import (
	"context"
	"time"
)

// StepID is a stable step identifier; it appears in logs and must not change once shipped.
type StepID string

// StepResult is a step's outcome from Orchestrator; skipped steps have
// Success=true and SkipReason set.
type StepResult struct {
	StepID     StepID
	Success    bool
	Error      error
	Skipped    bool
	SkipReason string
	StartedAt  time.Time
	Duration   time.Duration
}

// StepDef is a data-driven step definition with required ID, Name, and Exec;
// AlreadyDone runs before Exec and skips the step when true.
type StepDef struct {
	ID          StepID
	Name        string
	NonFatal    bool
	AlreadyDone func(ctx context.Context) (bool, error)
	SkipWhen    func() bool
	SkipReason  string
	// SkipReasonFunc overrides SkipReason after SkipWhen fires — use when
	// SkipWhen folds several causes into one.
	SkipReasonFunc func() string
	OnStart        func()
	Exec           func(ctx context.Context) error
	OnError        func(error)
}

// BuildSteps validates defs for NewOrchestrator. Panics on an empty ID or Name.
func BuildSteps(defs []StepDef) []StepDef {
	for i := range defs {
		if defs[i].ID == "" {
			panic("BuildSteps: step has empty ID")
		}
		if defs[i].Name == "" {
			panic("BuildSteps: step " + string(defs[i].ID) + " has empty Name")
		}
	}
	return defs
}

func (d *StepDef) skipReason() string {
	if d.SkipReasonFunc != nil {
		return d.SkipReasonFunc()
	}
	return d.SkipReason
}
