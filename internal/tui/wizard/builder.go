package wizard

import (
	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
)

// ===============================================================================
// STEP BUILDER
// ===============================================================================

// StepBuilder creates wizard steps using registered factories.
type StepBuilder struct {
	factories map[StepType]StepBuilderFactory
}

// StepBuilderFactory creates a wizard step and returns initialization state.
type StepBuilderFactory func() (WizardStep, any)

// StepInitializer sets initial values on a step from config.
type StepInitializer func(step WizardStep, state any, cfg *config.Config)

// NewStepBuilder creates a new step builder.
func NewStepBuilder() *StepBuilder {
	return &StepBuilder{
		factories: make(map[StepType]StepBuilderFactory),
	}
}

func (b *StepBuilder) Register(stepType StepType, factory StepBuilderFactory) {
	b.factories[stepType] = factory
}

// Build creates a wizard step of the given type, or nil if not registered.
func (b *StepBuilder) Build(stepType StepType) (WizardStep, any) {
	if factory, ok := b.factories[stepType]; ok {
		return factory()
	}
	return nil, nil
}

// ===============================================================================
// BUILT STEPS RESULT
// ===============================================================================

// BuiltSteps holds the result of building wizard steps from config.
type BuiltSteps struct {
	Steps  []WizardStep
	States map[StepType]any
}

// BuildSteps creates wizard steps from the given configuration.
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
