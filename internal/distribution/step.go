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

// NewStepBuilder returns a builder for a fatal-by-default step.
// Panics on empty id or name — these are always literal strings.
func NewStepBuilder(id StepID, name string) *StepBuilder {
	if id == "" {
		panic("NewStepBuilder: id must not be empty")
	}
	if name == "" {
		panic("NewStepBuilder: name must not be empty")
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
		return nil, fmt.Errorf("StepBuilder is nil")
	}
	return &builtStep{builder: b}, nil
}

// MustBuild is Build without error return; it panics if the builder is
// invalid. Use it only for steps constructed from literal id/name pairs
// at init or package-load time — callers that build steps from user
// input should use Build so validation errors surface normally.
func (b *StepBuilder) MustBuild() ProvisioningStep {
	step, err := b.Build()
	if err != nil {
		panic(err)
	}
	return step
}

// StepDef is a data-driven step definition. ID, Name, and Exec are required;
// everything else is optional. Fatal is the default — set NonFatal to true
// for steps that should log a warning on failure and continue.
type StepDef struct {
	ID         StepID
	Name       string
	Desc       string
	NonFatal   bool
	SkipWhen   func() bool
	SkipReason string
	OnStart    func()
	Exec       func(ctx context.Context) error
	OnError    func(error)
}

// BuildSteps converts a slice of StepDef into ProvisioningSteps ready for
// NewOrchestrator. Panics via MustBuild if any StepDef has an empty ID or Name.
// Fatal is set explicitly from !NonFatal so the guarantee does not depend on
// NewStepBuilder's default — if that default ever changes, this helper still
// produces the correct behavior.
func BuildSteps(defs []StepDef) []ProvisioningStep {
	steps := make([]ProvisioningStep, 0, len(defs))
	for _, d := range defs {
		b := NewStepBuilder(d.ID, d.Name).Description(d.Desc).Fatal(!d.NonFatal)
		if d.SkipWhen != nil {
			b = b.SkipWhen(d.SkipWhen).SkipReason(d.SkipReason)
		}
		if d.OnStart != nil {
			b = b.OnStart(d.OnStart)
		}
		b = b.Execute(d.Exec)
		if d.OnError != nil {
			b = b.OnError(d.OnError)
		}
		steps = append(steps, b.MustBuild())
	}
	return steps
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
