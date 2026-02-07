// Package distribution provides step-based provisioning orchestration.
package distribution

import (
	"context"
	"fmt"
)

// StepID identifies a provisioning step.
type StepID string

// StepResult contains the outcome of a step execution.
type StepResult struct {
	StepID     StepID
	Success    bool
	Error      error
	Skipped    bool
	SkipReason string
}

// Step is the minimal interface for a provisioning step.
type Step interface {
	ID() StepID
	Name() string
	Description() string
	Execute(ctx context.Context) error
}

// Skipper allows steps to be conditionally skipped.
type Skipper interface {
	ShouldSkip() bool
	SkipReason() string
}

// FatalChecker indicates if a step failure is fatal.
type FatalChecker interface {
	IsFatal() bool
}

// StepCallbacks provides lifecycle hooks.
type StepCallbacks interface {
	OnStart()
	OnComplete()
	OnError(err error)
}

// ProvisioningStep combines all step interfaces.
type ProvisioningStep interface {
	Step
	Skipper
	FatalChecker
	StepCallbacks
}

// ═══════════════════════════════════════════════════════════════════════════════
// STEP BUILDER
// ═══════════════════════════════════════════════════════════════════════════════

// StepBuilder provides a fluent API for constructing provisioning steps.
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

// NewStepBuilder creates a new step builder with the given ID and name.
// Both id and name are required and must be non-empty.
// Returns nil if id or name are empty - callers should use MustNewStepBuilder
// for compile-time constants where panicking is acceptable.
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

// MustNewStepBuilder creates a new step builder, panicking if id or name are empty.
// Use this for compile-time constants where empty values indicate a programming error.
func MustNewStepBuilder(id StepID, name string) *StepBuilder {
	if id == "" {
		panic("MustNewStepBuilder: id cannot be empty")
	}
	if name == "" {
		panic("MustNewStepBuilder: name cannot be empty")
	}
	return &StepBuilder{
		id:    id,
		name:  name,
		fatal: true,
	}
}

// Description sets the step description.
func (b *StepBuilder) Description(d string) *StepBuilder {
	b.description = d
	return b
}

// Fatal sets whether step failure is fatal.
func (b *StepBuilder) Fatal(f bool) *StepBuilder {
	b.fatal = f
	return b
}

// SkipWhen sets a function to determine if the step should be skipped.
func (b *StepBuilder) SkipWhen(fn func() bool) *StepBuilder {
	b.skipFn = fn
	return b
}

// SkipReason sets the reason for skipping the step.
func (b *StepBuilder) SkipReason(r string) *StepBuilder {
	b.skipReason = r
	return b
}

// OnStart sets a callback to be invoked when the step starts.
func (b *StepBuilder) OnStart(fn func()) *StepBuilder {
	b.onStart = fn
	return b
}

// OnComplete sets a callback to be invoked when the step completes.
func (b *StepBuilder) OnComplete(fn func()) *StepBuilder {
	b.onComplete = fn
	return b
}

// OnError sets a callback to be invoked when the step fails.
func (b *StepBuilder) OnError(fn func(error)) *StepBuilder {
	b.onError = fn
	return b
}

// Execute sets the function that performs the step's main work.
func (b *StepBuilder) Execute(fn func(context.Context) error) *StepBuilder {
	b.executeFn = fn
	return b
}

// Build creates the configured step.
// Returns an error if ID or Name are not set.
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

// MustBuild creates the configured step, panicking on error.
// Use this for compile-time constants where invalid configuration is a programming error.
func (b *StepBuilder) MustBuild() ProvisioningStep {
	step, err := b.Build()
	if err != nil {
		panic(err)
	}
	return step
}

// builtStep implements ProvisioningStep using builder configuration.
type builtStep struct {
	builder *StepBuilder
}

// ID returns the step identifier.
func (s *builtStep) ID() StepID { return s.builder.id }

// Name returns the step name.
func (s *builtStep) Name() string { return s.builder.name }

// Description returns the step description.
func (s *builtStep) Description() string { return s.builder.description }

// IsFatal returns whether errors should stop execution.
func (s *builtStep) IsFatal() bool { return s.builder.fatal }

// ShouldSkip returns true if the step should be skipped.
func (s *builtStep) ShouldSkip() bool {
	if s.builder.skipFn == nil {
		return false
	}
	return s.builder.skipFn()
}

// SkipReason returns the reason for skipping.
func (s *builtStep) SkipReason() string { return s.builder.skipReason }

// Execute runs the step's main work.
func (s *builtStep) Execute(ctx context.Context) error {
	if s.builder.executeFn == nil {
		return nil
	}
	return s.builder.executeFn(ctx)
}

// OnStart is called when the step begins.
func (s *builtStep) OnStart() {
	if s.builder.onStart != nil {
		s.builder.onStart()
	}
}

// OnComplete is called when the step completes successfully.
func (s *builtStep) OnComplete() {
	if s.builder.onComplete != nil {
		s.builder.onComplete()
	}
}

// OnError is called when the step fails.
func (s *builtStep) OnError(err error) {
	if s.builder.onError != nil {
		s.builder.onError(err)
	}
}
