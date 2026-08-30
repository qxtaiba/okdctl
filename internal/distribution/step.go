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

// ReRunSafety declares whether a step may re-run on a fresh orchestrator run;
// BuildSteps panics on the zero value.
type ReRunSafety int8

// ReRunSafe values declare whether a step body may be re-executed mid-phase.
const (
	ReRunSafeUnset ReRunSafety = 0
	ReRunSafeYes   ReRunSafety = 1
	ReRunSafeNo    ReRunSafety = 2
)

// StepDef is a data-driven step definition with required ID, Name, Exec, and
// ReRunSafe; AlreadyDone runs before Exec and skips the step when true.
type StepDef struct {
	ID          StepID
	Name        string
	NonFatal    bool
	ReRunSafe   ReRunSafety
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

// builtStep is the runtime step: BuildSteps is its only constructor,
// Orchestrator its only consumer.
type builtStep struct {
	id           StepID
	name         string
	fatal        bool
	alreadyDone  func(context.Context) (bool, error)
	skipWhen     func() bool
	skipReason   string
	skipReasonFn func() string
	onStart      func()
	onError      func(error)
	exec         func(context.Context) error
}

// BuildSteps converts StepDefs into steps for NewOrchestrator. Panics on an
// empty ID/Name, ReRunSafeUnset, or ReRunSafeNo without AlreadyDone.
func BuildSteps(defs []StepDef) []*builtStep {
	steps := make([]*builtStep, 0, len(defs))
	for _, d := range defs {
		if d.ReRunSafe == ReRunSafeUnset {
			panic("BuildSteps: step " + string(d.ID) + " must declare ReRunSafe (ReRunSafeYes or ReRunSafeNo)")
		}
		if d.ReRunSafe == ReRunSafeNo && d.AlreadyDone == nil {
			panic("BuildSteps: step " + string(d.ID) + " is ReRunSafeNo but has no AlreadyDone guard")
		}
		if d.ID == "" {
			panic("BuildSteps: step has empty ID")
		}
		if d.Name == "" {
			panic("BuildSteps: step " + string(d.ID) + " has empty Name")
		}
		steps = append(steps, &builtStep{
			id:           d.ID,
			name:         d.Name,
			fatal:        !d.NonFatal,
			alreadyDone:  d.AlreadyDone,
			skipWhen:     d.SkipWhen,
			skipReason:   d.SkipReason,
			skipReasonFn: d.SkipReasonFunc,
			onStart:      d.OnStart,
			exec:         d.Exec,
			onError:      d.OnError,
		})
	}
	return steps
}

func (s *builtStep) ID() StepID { return s.id }

func (s *builtStep) Name() string { return s.name }

func (s *builtStep) IsFatal() bool { return s.fatal }

func (s *builtStep) IsAlreadyDone(ctx context.Context) (bool, error) {
	if s.alreadyDone == nil {
		return false, nil
	}
	return s.alreadyDone(ctx)
}

func (s *builtStep) ShouldSkip() bool {
	if s.skipWhen == nil {
		return false
	}
	return s.skipWhen()
}

func (s *builtStep) SkipReason() string {
	if s.skipReasonFn != nil {
		return s.skipReasonFn()
	}
	return s.skipReason
}

func (s *builtStep) Execute(ctx context.Context) error {
	if s.exec == nil {
		return nil
	}
	return s.exec(ctx)
}

func (s *builtStep) OnStart() {
	if s.onStart != nil {
		s.onStart()
	}
}

func (s *builtStep) OnError(err error) {
	if s.onError != nil {
		s.onError(err)
	}
}
