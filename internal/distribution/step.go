// Package distribution provides step-based provisioning orchestration.
package distribution

import (
	"context"
	"fmt"
)

type StepID string

type StepResult struct {
	StepID     StepID
	Success    bool
	Error      error
	Skipped    bool
	SkipReason string
}

type Step interface {
	ID() StepID
	Name() string
	Description() string
	Execute(ctx context.Context) error
}

type Skipper interface {
	ShouldSkip() bool
	SkipReason() string
}

type FatalChecker interface {
	IsFatal() bool
}

type StepCallbacks interface {
	OnStart()
	OnComplete()
	OnError(err error)
}

type ProvisioningStep interface {
	Step
	Skipper
	FatalChecker
	StepCallbacks
}

type StepBuilder struct {
	id          StepID
	name        string
	description string
	fatal       bool
	skipFn      func() bool
	skipReason  string
	onStart     func()
	onComplete  func()
	onError     func(error)
	executeFn   func(context.Context) error
}

// NewStepBuilder creates a new step builder. Returns nil if id or name are empty.
func NewStepBuilder(id StepID, name string) *StepBuilder {
	if id == "" || name == "" {
		return nil
	}
	return &StepBuilder{
		id:    id,
		name:  name,
		fatal: true, // default to fatal
	}
}

func (b *StepBuilder) Description(d string) *StepBuilder {
	b.description = d
	return b
}

func (b *StepBuilder) Fatal(f bool) *StepBuilder {
	b.fatal = f
	return b
}

func (b *StepBuilder) SkipWhen(fn func() bool) *StepBuilder {
	b.skipFn = fn
	return b
}

func (b *StepBuilder) SkipReason(r string) *StepBuilder {
	b.skipReason = r
	return b
}

func (b *StepBuilder) OnStart(fn func()) *StepBuilder {
	b.onStart = fn
	return b
}

func (b *StepBuilder) OnComplete(fn func()) *StepBuilder {
	b.onComplete = fn
	return b
}

func (b *StepBuilder) OnError(fn func(error)) *StepBuilder {
	b.onError = fn
	return b
}

func (b *StepBuilder) Execute(fn func(context.Context) error) *StepBuilder {
	b.executeFn = fn
	return b
}

func (b *StepBuilder) Build() (ProvisioningStep, error) {
	if b == nil {
		return nil, fmt.Errorf("StepBuilder is nil - NewStepBuilder returned nil due to empty id or name")
	}
	if b.id == "" {
		return nil, fmt.Errorf("StepBuilder: ID is required - call NewStepBuilder with a non-empty ID")
	}
	if b.name == "" {
		return nil, fmt.Errorf("StepBuilder: Name is required - call NewStepBuilder with a non-empty name")
	}
	return &builtStep{builder: b}, nil
}

func (b *StepBuilder) MustBuild() ProvisioningStep {
	step, err := b.Build()
	if err != nil {
		panic(err)
	}
	return step
}

type builtStep struct {
	builder *StepBuilder
}

func (s *builtStep) ID() StepID { return s.builder.id }

func (s *builtStep) Name() string { return s.builder.name }

func (s *builtStep) Description() string { return s.builder.description }

func (s *builtStep) IsFatal() bool { return s.builder.fatal }

func (s *builtStep) ShouldSkip() bool {
	if s.builder.skipFn == nil {
		return false
	}
	return s.builder.skipFn()
}

func (s *builtStep) SkipReason() string { return s.builder.skipReason }

func (s *builtStep) Execute(ctx context.Context) error {
	if s.builder.executeFn == nil {
		return nil
	}
	return s.builder.executeFn(ctx)
}

func (s *builtStep) OnStart() {
	if s.builder.onStart != nil {
		s.builder.onStart()
	}
}

func (s *builtStep) OnComplete() {
	if s.builder.onComplete != nil {
		s.builder.onComplete()
	}
}

func (s *builtStep) OnError(err error) {
	if s.builder.onError != nil {
		s.builder.onError(err)
	}
}
