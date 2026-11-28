package wizard

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
)

// RunFlow runs the wizard until the user completes or cancels it; ctx owns
// cancellation, so a signal surfaces as an error wrapping context.Canceled.
func RunFlow(ctx context.Context, steps []WizardStep, cfg *config.Config, chrome FlowChrome) (Result, error) {
	model := NewFlowModel(steps, cfg, chrome)
	model.flowContext = ctx
	defer model.shutdown()

	// root.go's signal loop cancels ctx; bubbletea's own handler would race
	// it for the signal and leave the terminal raw when it lost.
	p := tea.NewProgram(model, tea.WithContext(ctx), tea.WithoutSignalHandler())
	finalModel, err := p.Run()
	if err != nil {
		return Result{}, fmt.Errorf("wizard: %w", err)
	}
	m, ok := finalModel.(*Model)
	if !ok {
		return Result{}, fmt.Errorf("unexpected model type returned from wizard: %T", finalModel)
	}
	return flowResult(m, steps), nil
}

func flowResult(m *Model, steps []WizardStep) Result {
	result := m.Result()
	if result.Outcome != OutcomeCompleted {
		result.Outcome = OutcomeCancelled
		return result
	}
	result.Config = m.Config()
	for _, step := range steps {
		if p, ok := step.(SecretsProvider); ok {
			result.Secrets = p.Secrets()
		}
	}
	return result
}
