package distribution

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

// MetricsRecorder receives per-step and overall-run observations from the
// Orchestrator. Implementations must be safe for concurrent use.
type MetricsRecorder interface {
	StepStarted(id StepID)
	StepFinished(result *StepResult)
	DeployFinished(total time.Duration)
}

type nopMetricsRecorder struct{}

func (nopMetricsRecorder) StepStarted(StepID)           {}
func (nopMetricsRecorder) StepFinished(*StepResult)     {}
func (nopMetricsRecorder) DeployFinished(time.Duration) {}

// Orchestrator runs a sequence of ProvisioningSteps, recording per-step
// outcomes. Stops on the first fatal failure (per Step.IsFatal); non-fatal
// failures log a warning and continue. Safe to snapshot Results concurrently
// with Run.
type Orchestrator struct {
	mu      sync.RWMutex
	steps   []ProvisioningStep
	results []StepResult
	logger  *slog.Logger
	rec     MetricsRecorder
}

// NewOrchestrator returns an Orchestrator seeded with the given steps and a
// NopLogger. Use SetLogger to attach a real logger before Run.
func NewOrchestrator(steps ...ProvisioningStep) *Orchestrator {
	return &Orchestrator{
		steps:   steps,
		results: make([]StepResult, 0, len(steps)),
		logger:  logutil.NopLogger,
		rec:     nopMetricsRecorder{},
	}
}

// SetLogger attaches a logger. Nil is tolerated and resolved to NopLogger
// via logutil.OrNop.
func (o *Orchestrator) SetLogger(logger *slog.Logger) {
	o.logger = logutil.OrNop(logger)
}

// SetMetricsRecorder attaches a metrics recorder. Nil resolves to the nop
// recorder so callers never need a nil guard.
func (o *Orchestrator) SetMetricsRecorder(rec MetricsRecorder) {
	if rec == nil {
		o.rec = nopMetricsRecorder{}
		return
	}
	o.rec = rec
}

// Run executes each step in order, honoring ctx cancellation between steps.
// Returns the first fatal-step error (or ctx.Err on cancel), nil otherwise.
// Per-step results are recorded in Results even on cancellation so callers
// can render a partial-progress summary.
func (o *Orchestrator) Run(ctx context.Context) error {
	o.mu.Lock()
	o.results = make([]StepResult, 0, len(o.steps))
	o.mu.Unlock()

	runStart := time.Now()

	for _, step := range o.steps {
		select {
		case <-ctx.Done():
			o.rec.DeployFinished(time.Since(runStart))
			return ctx.Err()
		default:
		}

		result := o.executeStep(ctx, step)
		o.mu.Lock()
		o.results = append(o.results, result)
		o.mu.Unlock()

		if result.Skipped {
			o.logger.Info("step: skipped", "step", step.ID(), "name", step.Name(), "reason", result.SkipReason)
			continue
		}

		if !result.Success && step.IsFatal() {
			o.rec.DeployFinished(time.Since(runStart))
			return result.Error
		}
	}
	o.rec.DeployFinished(time.Since(runStart))
	return nil
}

// Results returns a snapshot of step results collected so far. Safe to call
// concurrently with Run; the returned slice is a copy.
func (o *Orchestrator) Results() []StepResult {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return slices.Clone(o.results)
}

// classifyStepErr wraps a bare step error in ClusterError so the cli layer
// maps it to exit 4 instead of the exit-1 default. Already-typed errtypes
// values and context cancellation/deadline errors are returned unchanged.
func classifyStepErr(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var (
		ce   *errtypes.ClusterError
		ne   *errtypes.NetworkError
		cfge *errtypes.ConfigError
		ae   *errtypes.AuthError
		ue   *errtypes.UsageError
	)
	if errors.As(err, &ce) || errors.As(err, &ne) || errors.As(err, &cfge) || errors.As(err, &ae) || errors.As(err, &ue) {
		return err
	}
	return &errtypes.ClusterError{Msg: "step failed", Err: err}
}

func (o *Orchestrator) executeStep(ctx context.Context, step ProvisioningStep) StepResult {
	startedAt := time.Now()

	if step.ShouldSkip() {
		r := StepResult{
			StepID:     step.ID(),
			Success:    true,
			Skipped:    true,
			SkipReason: step.SkipReason(),
			StartedAt:  startedAt,
			Duration:   time.Since(startedAt),
		}
		o.rec.StepFinished(&r)
		return r
	}

	if checker, ok := step.(AlreadyDoneChecker); ok {
		done, err := checker.IsAlreadyDone(ctx)
		if err != nil {
			o.logger.Warn("step: already-done check failed, proceeding", "step", step.ID(), "err", err)
		} else if done {
			r := StepResult{
				StepID:     step.ID(),
				Success:    true,
				Skipped:    true,
				SkipReason: "already done",
				StartedAt:  startedAt,
				Duration:   time.Since(startedAt),
			}
			o.logger.Info("step: skipped (already done)", "step", step.ID(), "name", step.Name(), "reason", "already done")
			o.rec.StepFinished(&r)
			return r
		}
	}

	o.rec.StepStarted(step.ID())
	o.logger.Info("step: started", "step", step.ID(), "name", step.Name())
	step.OnStart()

	if err := step.Execute(ctx); err != nil {
		err = classifyStepErr(err)
		step.OnError(err)
		r := StepResult{
			StepID:    step.ID(),
			Success:   false,
			Error:     err,
			StartedAt: startedAt,
			Duration:  time.Since(startedAt),
		}
		// Warn here; the cli layer Errors once on command failure (double-log avoidance).
		o.logger.Warn("step: failed", "step", step.ID(), "duration", r.Duration, "fatal", step.IsFatal(), "err", err)
		o.rec.StepFinished(&r)
		return r
	}

	step.OnComplete()
	r := StepResult{
		StepID:    step.ID(),
		Success:   true,
		StartedAt: startedAt,
		Duration:  time.Since(startedAt),
	}
	o.logger.Info("step: succeeded", "step", step.ID(), "duration", r.Duration)
	o.rec.StepFinished(&r)
	return r
}
