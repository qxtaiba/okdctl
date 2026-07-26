package wizard

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
)

// Run starts the bubbletea wizard with the default chrome and blocks until
// the user completes or cancels the flow.
func Run(ctx context.Context, steps []WizardStep, cfg *config.Config) (Result, error) {
	return RunFlow(ctx, steps, cfg, DefaultChrome())
}

// RunFlow starts the bubbletea wizard with flow-specific chrome and blocks
// until the user completes or cancels the flow.
func RunFlow(ctx context.Context, steps []WizardStep, cfg *config.Config, chrome FlowChrome) (Result, error) {
	model := NewFlowModel(steps, cfg, chrome)

	p := tea.NewProgram(model,
		tea.WithContext(ctx),
	)
	finalModel, err := p.Run()
	if err != nil {
		return Result{}, fmt.Errorf("wizard error: %w", err)
	}

	m, ok := finalModel.(*Model)
	if !ok {
		return Result{}, fmt.Errorf("unexpected model type returned from wizard: %T", finalModel)
	}

	result := m.Result()
	if result.Completed {
		result.Config = m.Config()
	}

	return result, nil
}
