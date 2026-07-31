// Package distribution hosts the phase-step orchestration primitives
// (StepDef, BuildSteps, Orchestrator) shared by every distribution under
// internal/distribution/. StepDef + BuildSteps is the only supported way to
// construct a step; Orchestrator consumes the package-private step type
// BuildSteps returns.
package distribution

import (
	"context"
	"time"
)

// StepID is a stable identifier for a provisioning step. IDs appear in logs,
// so they must not change once a step ships.
type StepID string

// StepResult is the outcome of a single provisioning step, populated by the
// Orchestrator. Skipped steps carry Success=true with SkipReason set.
type StepResult struct {
	StepID     StepID
	Success    bool
	Error      error
	Skipped    bool
	SkipReason string
	StartedAt  time.Time
	Duration   time.Duration
}

// ReRunSafety declares whether a step is safe to re-execute on a fresh
// orchestrator run. BuildSteps panics on the zero value (ReRunSafeUnset) so
// every StepDef must commit to one or the other.
type ReRunSafety int8

// ReRunSafe values declare whether a step body may be re-executed mid-phase.
// ReRunSafeUnset is the zero value and triggers the BuildSteps panic so every
// StepDef literal must commit to ReRunSafeYes or ReRunSafeNo.
const (
	ReRunSafeUnset ReRunSafety = 0
	ReRunSafeYes   ReRunSafety = 1
	ReRunSafeNo    ReRunSafety = 2
)

// StepDef is a data-driven step definition. ID, Name, Exec, and ReRunSafe are
// required; everything else is optional. Fatal is the default — set NonFatal
// to true for steps that should log a warning on failure and continue.
//
// AlreadyDone is consulted before Exec when present; a true return records
// the step as Skipped. Useful for ReRunSafeNo steps that can detect their
// work product after a partial-fail-and-resume.
type StepDef struct {
	ID          StepID
	Name        string
	Desc        string
	NonFatal    bool
	ReRunSafe   ReRunSafety
	AlreadyDone func(ctx context.Context) (bool, error)
	SkipWhen    func() bool
	SkipReason  string
	// SkipReasonFunc resolves the skip reason after SkipWhen fires and wins
	// over SkipReason when set. Use it when SkipWhen folds several causes and
	// the logged reason must name the one that actually fired.
	SkipReasonFunc func() string
	OnStart        func()
	Exec           func(ctx context.Context) error
	OnError        func(error)
}

// builtStep is the runtime representation of a single provisioning step.
// BuildSteps is the only constructor and Orchestrator is the only consumer —
// there is no other implementation, so the former Step/Skipper/FatalChecker/
// StepCallbacks role-interface split added indirection without an extension
// point anyone used.
type builtStep struct {
	id           StepID
	name         string
	description  string
	fatal        bool
	reRunSafe    ReRunSafety
	alreadyDone  func(context.Context) (bool, error)
	skipWhen     func() bool
	skipReason   string
	skipReasonFn func() string
	onStart      func()
	onComplete   func()
	onError      func(error)
	exec         func(context.Context) error
}

// BuildSteps converts a slice of StepDef into steps ready for
// NewOrchestrator. Panics when any StepDef has an empty ID, empty Name,
// ReRunSafe == ReRunSafeUnset, or ReRunSafe == ReRunSafeNo with a nil
// AlreadyDone — every ReRunSafeNo step must provide a precondition guard.
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
			description:  d.Desc,
			fatal:        !d.NonFatal,
			reRunSafe:    d.ReRunSafe,
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

func (s *builtStep) Description() string { return s.description }

func (s *builtStep) IsFatal() bool { return s.fatal }

// ReRunSafe returns the idempotency declaration for this step, propagated
// from StepDef.ReRunSafe by BuildSteps. A future Orchestrator change may
// branch on this to skip ReRunSafeNo steps on recovery reruns.
func (s *builtStep) ReRunSafe() ReRunSafety { return s.reRunSafe }

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

func (s *builtStep) OnComplete() {
	if s.onComplete != nil {
		s.onComplete()
	}
}

func (s *builtStep) OnError(err error) {
	if s.onError != nil {
		s.onError(err)
	}
}
