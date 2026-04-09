package wizard

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
)

func Run(ctx context.Context, steps []WizardStep, cfg *config.Config) (Result, error) {
	model := NewModel(steps, cfg)

	p := tea.NewProgram(model,
		tea.WithContext(ctx),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
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
