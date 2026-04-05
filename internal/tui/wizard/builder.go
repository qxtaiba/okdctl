// Package wizard implements the bubbletea model and step orchestration for
// openshitctl's interactive configuration wizard, producing a validated
// config.Config for downstream deployment.
package wizard

import (
	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
)

type StepBuilder struct {
	factories map[StepType]StepBuilderFactory
}

type StepBuilderFactory func() (WizardStep, any)

type StepInitializer func(step WizardStep, state any, cfg *config.Config)

func NewStepBuilder() *StepBuilder {
	return &StepBuilder{
		factories: make(map[StepType]StepBuilderFactory),
	}
}

func (b *StepBuilder) Register(stepType StepType, factory StepBuilderFactory) {
	b.factories[stepType] = factory
}

func (b *StepBuilder) Build(stepType StepType) (WizardStep, any) {
	if factory, ok := b.factories[stepType]; ok {
		return factory()
	}
	return nil, nil
}

type BuiltSteps struct {
	Steps  []WizardStep
	States map[StepType]any
}

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
