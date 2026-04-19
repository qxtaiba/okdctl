package distribution

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

// Orchestrator runs a sequence of ProvisioningSteps, recording per-step
// outcomes. Stops on the first fatal failure (per Step.IsFatal); non-fatal
// failures log a warning and continue. Safe to snapshot Results concurrently
// with Run.
type Orchestrator struct {
	mu      sync.RWMutex
	steps   []ProvisioningStep
	results []StepResult
	logger  *slog.Logger
}

// NewOrchestrator returns an Orchestrator seeded with the given steps and a
// NopLogger. Use SetLogger to attach a real logger before Run.
func NewOrchestrator(steps ...ProvisioningStep) *Orchestrator {
	return &Orchestrator{
		steps:   steps,
		results: make([]StepResult, 0, len(steps)),
		logger:  logutil.NopLogger,
	}
}

// SetLogger attaches a logger. Nil is tolerated and resolved to NopLogger
// via logutil.OrNop.
func (o *Orchestrator) SetLogger(logger *slog.Logger) {
	o.logger = logutil.OrNop(logger)
}

// Run executes each step in order, honoring ctx cancellation between steps.
// Returns the first fatal-step error (or ctx.Err on cancel), nil otherwise.
// Per-step results are recorded in Results even on cancellation so callers
// can render a partial-progress summary.
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
			o.logger.Info("step: skipped", "step", step.Name(), "reason", result.SkipReason)
			continue
		}

		if !result.Success && step.IsFatal() {
			return result.Error
		}
	}
	return nil
}

// Results returns a snapshot of step results collected so far. Safe to call
// concurrently with Run; the returned slice is a copy.
func (o *Orchestrator) Results() []StepResult {
	o.mu.RLock()
	defer o.mu.RUnlock()
	out := make([]StepResult, len(o.results))
	copy(out, o.results)
	return out
}

func (o *Orchestrator) executeStep(ctx context.Context, step ProvisioningStep) StepResult {
	startedAt := time.Now()

	if step.ShouldSkip() {
		return StepResult{
			StepID:     step.ID(),
			Success:    true,
			Skipped:    true,
			SkipReason: step.SkipReason(),
			StartedAt:  startedAt,
			Duration:   time.Since(startedAt),
		}
	}

	step.OnStart()

	if err := step.Execute(ctx); err != nil {
		step.OnError(err)
		return StepResult{
			StepID:    step.ID(),
			Success:   false,
			Error:     err,
			StartedAt: startedAt,
			Duration:  time.Since(startedAt),
		}
	}

	step.OnComplete()
	return StepResult{
		StepID:    step.ID(),
		Success:   true,
		StartedAt: startedAt,
		Duration:  time.Since(startedAt),
	}
}
