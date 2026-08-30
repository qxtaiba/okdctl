package distribution

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

// MetricsRecorder receives per-step/run observations from Orchestrator;
// implementations must be concurrency-safe.
type MetricsRecorder interface {
	StepStarted(id StepID)
	StepFinished(result *StepResult)
	DeployFinished(total time.Duration)
}

type nopMetricsRecorder struct{}

func (nopMetricsRecorder) StepStarted(StepID)           {}
func (nopMetricsRecorder) StepFinished(*StepResult)     {}
func (nopMetricsRecorder) DeployFinished(time.Duration) {}

// stepLogSuppressor: a recorder with its own step narration returns true to
// demote Orchestrator's Info logs to Debug, avoiding double narration.
type stepLogSuppressor interface {
	SuppressStepLog() bool
}

// Orchestrator runs steps from BuildSteps, stopping at the first fatal
// failure (StepDef.NonFatal); Results can be read concurrently with Run.
type Orchestrator struct {
	mu           sync.RWMutex
	steps        []*builtStep
	results      []StepResult
	logger       *slog.Logger
	rec          MetricsRecorder
	suppressStep bool
}

// NewOrchestrator returns an Orchestrator with steps and a NopLogger; call
// SetLogger before Run to attach a real logger.
func NewOrchestrator(steps ...*builtStep) *Orchestrator {
	return &Orchestrator{
		steps:   steps,
		results: make([]StepResult, 0, len(steps)),
		logger:  logutil.NopLogger,
		rec:     nopMetricsRecorder{},
	}
}

// SetLogger attaches a logger, resolving nil to NopLogger via logutil.OrNop.
func (o *Orchestrator) SetLogger(logger *slog.Logger) {
	o.logger = logutil.OrNop(logger)
}

// SetMetricsRecorder attaches a recorder, resolving nil to a nop implementation.
func (o *Orchestrator) SetMetricsRecorder(rec MetricsRecorder) {
	if rec == nil {
		o.rec = nopMetricsRecorder{}
		o.suppressStep = false
		return
	}
	o.rec = rec
	if s, ok := rec.(stepLogSuppressor); ok {
		o.suppressStep = s.SuppressStepLog()
	} else {
		o.suppressStep = false
	}
}

// stepInfo logs at Info, or Debug when the recorder self-narrates — demoted
// lines still reach okdctl.log via the recorder's sink mirror.
func (o *Orchestrator) stepInfo(msg string, args ...any) {
	if o.suppressStep {
		o.logger.Debug(msg, args...)
		return
	}
	o.logger.Info(msg, args...)
}

// Run executes each step in order, honoring ctx cancellation; it returns the
// first fatal-step error (or ctx.Err) and records results even on cancellation.
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
			o.stepInfo("step: skipped", "step", step.ID(), "name", step.Name(), "reason", result.SkipReason)
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

// Results returns a snapshot copy of results so far; safe to call concurrently with Run.
func (o *Orchestrator) Results() []StepResult {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return slices.Clone(o.results)
}

// classifyStepErr wraps a bare error in ClusterError (exit 4, not exit 1);
// typed errtypes and ctx cancel/deadline errors pass through unchanged.
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
	// Msg bypasses RedactHandler, so err.Error() is laundered through
	// RedactableStderr before landing in Msg; Err stays untouched for errors.Is/As.
	scrubbed := fmt.Sprint(logutil.RedactableStderr(err.Error()).Redacted())
	return &errtypes.ClusterError{Msg: "step failed: " + scrubbed, Err: err}
}

func (o *Orchestrator) executeStep(ctx context.Context, step *builtStep) StepResult {
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

	done, err := step.IsAlreadyDone(ctx)
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
		o.stepInfo("step: skipped (already done)", "step", step.ID(), "name", step.Name(), "reason", "already done")
		o.rec.StepFinished(&r)
		return r
	}

	o.rec.StepStarted(step.ID())
	o.stepInfo("step: started", "step", step.ID(), "name", step.Name())
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

	r := StepResult{
		StepID:    step.ID(),
		Success:   true,
		StartedAt: startedAt,
		Duration:  time.Since(startedAt),
	}
	o.stepInfo("step: succeeded", "step", step.ID(), "duration", r.Duration)
	o.rec.StepFinished(&r)
	return r
}
