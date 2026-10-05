package wizard

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
)

// RunFlow starts the bubbletea wizard with flow-specific chrome and blocks
// until the user completes or cancels the flow.
func RunFlow(ctx context.Context, steps []WizardStep, cfg *config.Config, chrome FlowChrome) (Result, error) {
	return RunFlowWithDraft(ctx, steps, cfg, chrome, nil)
}

// RunFlowWithDraft runs a wizard and saves its cursor after each step transition.
func RunFlowWithDraft(ctx context.Context, steps []WizardStep, cfg *config.Config, chrome FlowChrome, save func(*config.Config, StepID, string) error) (Result, error) {
	model := NewFlowModel(steps, cfg, chrome)
	model.draftSaver = save
	model.flowContext = ctx
	defer model.shutdown()

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
	if result.Outcome == OutcomeCompleted {
		result.Config = m.Config()
	}

	return result, nil
}
