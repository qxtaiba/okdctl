package wizard

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

func Run(steps []WizardStep, cfg *config.Config) (Result, error) {
	model := NewModel(steps, cfg)

	p := tea.NewProgram(model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	finalModel, err := p.Run()
	if err != nil {
		return Result{}, utils.WrapError("wizard error", err)
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

type WizardResult = Result
type WizardAction = Action
