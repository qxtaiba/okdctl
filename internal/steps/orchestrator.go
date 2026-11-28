package steps

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"sync"
	"time"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

// Orchestrator runs steps from BuildSteps, stopping at the first fatal
// failure (StepDef.NonFatal); Results can be read concurrently with Run.
type Orchestrator struct {
	mu      sync.RWMutex
	steps   []StepDef
	results []StepResult
	rep     Reporter
}

// NewOrchestrator returns an Orchestrator streaming to rep; nil rep discards
// the stream.
func NewOrchestrator(rep Reporter, steps ...StepDef) *Orchestrator {
	if rep == nil {
		rep = nopReporter{}
	}
	return &Orchestrator{
		steps:   steps,
		results: make([]StepResult, 0, len(steps)),
		rep:     rep,
	}
}

// Run executes each step in order, honoring ctx cancellation; it returns the
// first fatal-step error (or ctx.Err) and records results even on cancellation.
func (o *Orchestrator) Run(ctx context.Context) error {
	o.mu.Lock()
	o.results = make([]StepResult, 0, len(o.steps))
	o.mu.Unlock()

	for i := range o.steps {
		step := &o.steps[i]
		if err := ctx.Err(); err != nil {
			return err
		}

		result := o.executeStep(ctx, step)
		o.mu.Lock()
		o.results = append(o.results, result)
		o.mu.Unlock()
		o.rep.StepFinished(&result)

		if !result.Success && !step.NonFatal {
			return result.Error
		}
	}
	return nil
}

// Results returns a snapshot copy of results so far; safe to call concurrently with Run.
func (o *Orchestrator) Results() []StepResult {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return slices.Clone(o.results)
}

// classifyStepErr types a bare step error by its chain: a transport failure
// is a NetworkError, anything else a ClusterError; typed errtypes errors (the
// driver's permission refusal among them) and ctx errors pass through unchanged.
func classifyStepErr(id StepID, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if _, ok := errtypes.Describe(err); ok {
		return err
	}
	// Msg bypasses RedactHandler, so err.Error() is laundered through
	// RedactableStderr before landing in Msg; Err stays untouched for errors.Is/As.
	msg := fmt.Sprintf("step %s failed: %v", id, logutil.RedactableStderr(err.Error()).Redacted())
	var netErr net.Error
	if errors.As(err, &netErr) {
		return &errtypes.NetworkError{Msg: msg, Err: err}
	}
	return &errtypes.ClusterError{Msg: msg, Err: err}
}

func (o *Orchestrator) executeStep(ctx context.Context, step *StepDef) (r StepResult) {
	r = StepResult{StepID: step.ID, StartedAt: time.Now()}
	defer func() { r.Duration = time.Since(r.StartedAt) }()

	if step.SkipWhen != nil && step.SkipWhen() {
		r.Success, r.Skipped, r.SkipReason = true, true, step.skipReason()
		return r
	}
	if step.AlreadyDone != nil {
		done, err := step.AlreadyDone(ctx)
		switch {
		case err != nil:
			o.rep.StepProgress(step.ID, "already-done check failed, proceeding: "+err.Error())
		case done:
			r.Success, r.Skipped, r.SkipReason = true, true, "already done"
			return r
		}
	}

	o.rep.StepStarted(step.ID, step.Name)
	if step.Exec != nil {
		if err := step.Exec(ctx); err != nil {
			r.Error = classifyStepErr(step.ID, err)
			return r
		}
	}
	r.Success = true
	return r
}
