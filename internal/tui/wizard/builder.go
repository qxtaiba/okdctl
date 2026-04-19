package wizard

import (
	"github.com/qxtaiba/okdctl/internal/config"
)

// StepBuilder is a registry of wizard step factories keyed by StepType.
type StepBuilder struct {
	factories map[StepType]StepBuilderFactory
}

// StepBuilderFactory constructs a WizardStep and its backing state.
type StepBuilderFactory func() (WizardStep, any)

// StepInitializer seeds a built step with values derived from cfg.
type StepInitializer func(step WizardStep, state any, cfg *config.Config)

// NewStepBuilder returns an empty StepBuilder ready for Register calls.
func NewStepBuilder() *StepBuilder {
	return &StepBuilder{
		factories: make(map[StepType]StepBuilderFactory),
	}
}

// Register binds a factory to stepType.
func (b *StepBuilder) Register(stepType StepType, factory StepBuilderFactory) {
	b.factories[stepType] = factory
}

// Build invokes the factory for stepType, returning (nil, nil) when unknown.
func (b *StepBuilder) Build(stepType StepType) (step WizardStep, state any) {
	if factory, ok := b.factories[stepType]; ok {
		return factory()
	}
	return nil, nil
}

// BuiltSteps is the result of BuildSteps: the ordered WizardSteps and their
// associated state values keyed by StepType.
type BuiltSteps struct {
	Steps  []WizardStep
	States map[StepType]any
}

// BuildSteps materializes the steps listed in wizardCfg using builder.
func BuildSteps(wizardCfg Config, builder *StepBuilder) BuiltSteps {
	result := BuiltSteps{
		Steps:  make([]WizardStep, 0, len(wizardCfg.Steps)),
		States: make(map[StepType]any),
	}

	for _, stepCfg := range wizardCfg.Steps {
		step, state := builder.Build(stepCfg.Type)
		if step != nil {
			result.Steps = append(result.Steps, step)
			if state != nil {
				result.States[stepCfg.Type] = state
			}
		}
	}

	return result
}
