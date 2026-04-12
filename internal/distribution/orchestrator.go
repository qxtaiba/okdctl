package distribution

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

type Orchestrator struct {
	mu      sync.RWMutex
	steps   []ProvisioningStep
	results []StepResult
	logger  *slog.Logger
}

func NewOrchestrator(steps ...ProvisioningStep) *Orchestrator {
	return &Orchestrator{
		steps:   steps,
		results: make([]StepResult, 0, len(steps)),
		logger:  logutil.NopLogger,
	}
}

func (o *Orchestrator) SetLogger(logger *slog.Logger) {
	if logger != nil {
		o.logger = logger
	}
}

func (o *Orchestrator) Run(ctx context.Context) error {
	o.mu.Lock()
	o.results = make([]StepResult, 0, len(o.steps))
	o.mu.Unlock()

	for _, step := range o.steps {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		result := o.executeStep(ctx, step)
		o.mu.Lock()
		o.results = append(o.results, result)
		o.mu.Unlock()

		if result.Skipped {
			o.logger.Info(fmt.Sprintf("skipping %s: %s", step.Name(), result.SkipReason))
			continue
		}

		if !result.Success && step.IsFatal() {
			return result.Error
		}
	}
	return nil
}

func (o *Orchestrator) executeStep(ctx context.Context, step ProvisioningStep) StepResult {
	if step.ShouldSkip() {
		return StepResult{
			StepID:     step.ID(),
			Success:    true,
			Skipped:    true,
			SkipReason: step.SkipReason(),
		}
	}

	step.OnStart()

	if err := step.Execute(ctx); err != nil {
		step.OnError(err)
		return StepResult{
			StepID:  step.ID(),
			Success: false,
			Error:   err,
		}
	}

	step.OnComplete()
	return StepResult{
		StepID:  step.ID(),
		Success: true,
	}
}
