package wizard

import (
	"github.com/qxtaiba/okdctl/internal/config"
)

// StepBuilder is a registry of wizard step factories keyed by StepType.
type StepBuilder struct {
	factories map[StepType]StepBuilderFactory
}

// StepState is a marker interface for a step factory's optional state value,
// preventing accidental registration of a type that fails only at runtime.
type StepState interface{ IsWizardStepState() }

// StepBuilderFactory constructs a WizardStep and its backing state.
type StepBuilderFactory func() (WizardStep, StepState)

// StepInitializer seeds a built step with values derived from cfg.
type StepInitializer func(step WizardStep, state StepState, cfg *config.Config)

// NewStepBuilder returns an empty StepBuilder. Call Register per step
// type, then BuildSteps to assemble a wizard.
func NewStepBuilder() *StepBuilder {
	return &StepBuilder{
		factories: make(map[StepType]StepBuilderFactory),
	}
}

// Register associates a factory with stepType. Later calls for the same
// stepType overwrite the previous factory.
func (b *StepBuilder) Register(stepType StepType, factory StepBuilderFactory) {
	b.factories[stepType] = factory
}

// Build invokes the factory for stepType, returning (nil, nil) when unknown.
func (b *StepBuilder) Build(stepType StepType) (step WizardStep, state StepState) {
	if factory, ok := b.factories[stepType]; ok {
		return factory()
	}
	return nil, nil
}

// BuiltSteps is the result of BuildSteps: the ordered WizardSteps and their
// associated state values keyed by StepType.
type BuiltSteps struct {
	Steps  []WizardStep
	States map[StepType]StepState
}

// BuildSteps builds the ordered step list and per-type state map from
// wizardCfg.Steps using builder's factories.
func BuildSteps(wizardCfg Config, builder *StepBuilder) BuiltSteps {
	result := BuiltSteps{
		Steps:  make([]WizardStep, 0, len(wizardCfg.Steps)),
		States: make(map[StepType]StepState),
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
