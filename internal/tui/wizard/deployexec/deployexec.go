// Package deployexec implements the deploy wizard flow: a phase checklist fed
// by the deploy engine's metrics-recorder seam, and the post-deploy summary or
// error card rendered in-frame once the run ends.
package deployexec

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/postinstall"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/logview"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// StepIDs for the deploy flow's two screens.
const (
	StepIDStream wizard.StepID = "deploy-stream"
	StepIDDone   wizard.StepID = "deploy-done"
)

// State is shared by pointer across both deploy steps — the role
// *config.Config plays for the configure wizard. The stream step writes the
// run's outcome into it; the done step renders from it.
type State struct {
	Cfg *config.Config
	// Plan is the checklist the stream step seeds its rows from, in execution
	// order (deploy.PlannedSteps).
	Plan  []tui.StepMeta
	RunID string

	// History is the per-step duration seed for the progress weight model
	// and the ETA (LoadStepHistory's result); nil on a first install, which
	// renders even weights and no ETA.
	History map[distribution.StepID]time.Duration

	// Started marks the engine goroutine began; Executed marks it returned —
	// the gap is an interrupted run.
	Started  bool
	Executed bool

	// Summary is the postinstall result the done screen's completion box reads.
	Summary *postinstall.Result
	// Steps is every step the engine executed, in order.
	Steps []distribution.StepResult

	Result  error
	Elapsed time.Duration

	// frozen is the phase checklist exactly as the run left it, handed over by
	// the stream screen on its final event so the incident report can show the
	// run's shape above the error card; frozenAt names the phase that was in
	// flight. Unexported because only this package's two screens share it.
	frozen   []phaseProgress
	frozenAt int
}

// Event is one progress event on the deploy stream's feed: a step transition
// (StepID set, Done marking the closing bracket), or the terminal event
// (Final) the Execute hook sends once the engine returns.
type Event struct {
	StepID  distribution.StepID
	Done    bool
	Skipped bool
	Took    time.Duration
	Err     error
	Final   bool
}

// NextFlow builds the steps and chrome a finish-screen verb enters, deferred
// until the verb is pressed so a deploy that exits straight away never pays
// for the manage-nodes flow's credential load and Proxmox probe.
type NextFlow func() ([]wizard.WizardStep, wizard.FlowChrome, error)

// FinishHooks groups the optional actions available after deployment.
type FinishHooks struct {
	ManageNodes NextFlow
	OpenConsole func() tea.Cmd
}

// Hooks are the CLI-supplied closures the deploy steps call into, so this
// package never imports the cli package's assembly code.
type Hooks struct {
	// Execute runs the deploy engine to completion, feeding events as steps
	// start and finish.
	Execute func(st *State, events chan<- Event) error
	// CancelDeploy requests a graceful cancel of the run in flight.
	CancelDeploy func()
	// Logs is the human log stream the log pane reads; nil leaves the pane to
	// the wizard's own context pane.
	Logs logview.Source
	// LogPath is the resolved path of the run-log sink that keeps every byte
	// the ring evicts; empty when no file sink is open, and no screen may then
	// point at one.
	LogPath string
	// Done is closed once the run's context is cancelled. The step's own final
	// send selects on it, so a force-quit never strands the engine goroutine on
	// a feed nobody drains; a nil channel simply never fires.
	Done   <-chan struct{}
	Finish *FinishHooks
}

// flowStepCount is how many screens NewSteps assembles, the step count the
// frame's split gate is evaluated against; TestFlowStepCountMatchesNewSteps
// pins it.
const flowStepCount = 2

// NewSteps assembles the deploy flow's ordered steps. Direct construction
// instead of a StepBuilder registry: the registry's indirection earns its keep
// only with multiple assembly sites.
func NewSteps(st *State, hooks Hooks) []wizard.WizardStep {
	return []wizard.WizardStep{
		NewStreamStep(st, hooks),
		NewDoneStep(st, hooks),
	}
}

// Stages returns the deploy flow's two-stage breadcrumb.
func Stages() []wizard.Stage {
	return []wizard.Stage{
		{Label: "install", Steps: []wizard.StepID{StepIDStream}},
		{Label: "done", Steps: []wizard.StepID{StepIDDone}},
	}
}

// Chrome returns the deploy flow's header chrome, pairing the install tagline
// and cluster badge with the fixed two-stage trail.
func Chrome() wizard.FlowChrome {
	return wizard.FlowChrome{
		Tagline: "installing okd over proxmox",
		Badge:   func(cfg *config.Config) string { return cfg.Cluster.Name },
		Trail:   wizard.StagesTrail(Stages()),
	}
}

// NewRecorder returns the deploy engine's metrics-recorder seam, translating
// every step transition into an Event. Each send aborts on ctx, so a quit
// never strands the engine's goroutine on a channel nobody drains.
func NewRecorder(ctx context.Context, events chan<- Event) distribution.MetricsRecorder {
	return &recorder{ctx: ctx, events: events}
}

type recorder struct {
	ctx    context.Context
	events chan<- Event
}

// StepStarted opens the step's row on the checklist.
func (r *recorder) StepStarted(id distribution.StepID) {
	r.send(Event{StepID: id})
}

// StepFinished closes the step's row with its outcome and duration.
func (r *recorder) StepFinished(res *distribution.StepResult) {
	r.send(Event{
		StepID:  res.StepID,
		Done:    true,
		Skipped: res.Skipped,
		Took:    res.Duration,
		Err:     res.Error,
	})
}

// DeployFinished is inert: the orchestrator calls it once per phase, so it
// cannot mark the run's end — the Execute hook's return does.
func (r *recorder) DeployFinished(time.Duration) {}

// send delivers ev, abandoning it only once ctx is gone AND the feed cannot
// accept it — biased, because the graceful cancel cancels this very ctx and
// a uniform select would drop transitions the screen is still draining; a
// quit with a dead receiver still exits promptly through the guard.
func (r *recorder) send(ev Event) {
	select {
	case r.events <- ev:
	default:
		select {
		case <-r.ctx.Done():
		case r.events <- ev:
		}
	}
}
