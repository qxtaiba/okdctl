package steps

import (
	"log/slog"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

// Reporter receives the step stream Orchestrator emits for the plain logger
// or the TUI: StepStarted then StepFinished for a step that runs, StepFinished
// alone for a skipped one, and StepProgress for a step's own lines in between.
// StepProgress is called from the step's goroutines, so implementations must
// be safe for concurrent use.
type Reporter interface {
	StepStarted(id StepID, name string)
	StepProgress(id StepID, line string)
	StepFinished(result *StepResult)
}

type nopReporter struct{}

func (nopReporter) StepStarted(StepID, string)  {}
func (nopReporter) StepProgress(StepID, string) {}
func (nopReporter) StepFinished(*StepResult)    {}

// LogReporter is the Reporter for a plain-logger run: one Info line per step
// event and Warn for a failure; nil resolves to logutil.NopLogger.
func LogReporter(l *slog.Logger) Reporter {
	return logReporter{logutil.OrNop(l)}
}

type logReporter struct{ l *slog.Logger }

func (r logReporter) StepStarted(id StepID, name string) {
	r.l.Info("step: started", "step", id, "name", name)
}

// StepProgress wraps line in RedactableStderr: installer output is free text
// the handler can scrub only through a Redacted() dispatch.
func (r logReporter) StepProgress(id StepID, line string) {
	r.l.Info("step: progress", "step", id, "line", logutil.RedactableStderr(line))
}

func (r logReporter) StepFinished(res *StepResult) {
	switch {
	case res.Skipped:
		r.l.Info("step: skipped", "step", res.StepID, "reason", res.SkipReason)
	case res.Error != nil:
		r.l.Warn("step: failed", "step", res.StepID, "duration", res.Duration, "err", res.Error)
	default:
		r.l.Info("step: succeeded", "step", res.StepID, "duration", res.Duration)
	}
}
