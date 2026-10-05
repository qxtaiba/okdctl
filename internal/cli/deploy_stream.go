package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/deploy"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/tui/logview"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/deployexec"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

// deployInterruptedMsg is shared so runDeployStream's tea-failure path and
// reportDeployStreamOutcome present identical guidance.
const deployInterruptedMsg = "deploy was interrupted mid-install; the deploy-state marker records the phase — re-run 'okdctl deploy' to resume, or 'okdctl deploy --fresh' to restart from scratch (wipes cluster credentials)"

// demoStreamStepDelay paces each demo deploy step so the stream screen has
// visible in-progress rows to screenshot instead of finishing instantly.
const demoStreamStepDelay = 120 * time.Millisecond

// deployStreamOptedOut reports whether this run has asked for the plain stderr
// checklist rather than the full-screen stream. --no-tui says so outright; --yes
// is a scripted deploy whose durable record is the operator's scrollback, which
// an AltScreen erases on exit.
func deployStreamOptedOut() bool {
	return deployNoTUI || deployYes
}

// deployStreamEnabled reports whether the deploy stream screen may own the
// terminal: the opt-outs decline it, and both stdio ends must be a terminal,
// the same gate every other interactive okdctl screen is defined by.
func deployStreamEnabled() bool {
	if deployStreamOptedOut() {
		return false
	}
	return term.IsTerminal(int(os.Stdout.Fd())) && term.IsTerminal(int(os.Stdin.Fd()))
}

// runDeployStream runs the deploy engine behind the wizard's stream screen: the
// checklist is seeded from the same plan the engine will execute, the engine's
// metrics-recorder seam feeds the screen, and the human log stream goes to the
// run log instead of the stderr the AltScreen owns.
func runDeployStream(ctx context.Context, cmd *cobra.Command, cfg *config.Config, opts *deploy.Options, out io.Writer) error {
	// Resolved before the log redirect so a stale-marker warning still reaches
	// the operator's scrollback rather than only the run log.
	plan := deploy.PlannedSteps(cfg, opts.ProjectRoot, opts.FreshDeploy)

	st := &deployexec.State{Cfg: cfg, Plan: plan, RunID: logutil.RunID()}
	loadDeployHistory(st, opts.ProjectRoot)
	ring := logview.NewRing(logview.DefaultCap)

	streamCtx, cancelStream := context.WithCancel(ctx)
	defer cancelStream()

	slot := &lifecycleSlot{}
	hooks := deployStreamSession(streamCtx, cancelStream, cfg, opts)
	followOn := deployFollowOnHooks(cmd, cfg, slot)
	hooks.Finish = followOn.Finish
	hooks.Logs = ring
	// The resolved sink path, so the screen's "full log" pointers name the
	// file this run actually writes — or nothing at all when no sink opened.
	hooks.LogPath = runLogPath

	// The stream screen owns the terminal from here on: no spinner or rewriting
	// checklist may paint beneath the AltScreen, and every log line goes to the
	// run log the screen reads back. Both are restored before anything is
	// printed, so the recap lands on a terminal the wizard has released.
	progressBars := logutil.ProgressBarsEnabled()
	logutil.SetProgressBarsEnabled(false)
	defer logutil.SetProgressBarsEnabled(progressBars)
	// The ring tees the human log stream into the pane on its way to the run
	// log, so nothing the operator sees on screen is missing from okdctl.log.
	restoreLogs := sync.OnceFunc(logutil.RedirectHandler(
		ring.Handler(slog.NewTextHandler(subprocSink(), nil))))
	defer restoreLogs()

	result, err := wizard.RunFlow(ctx, deployexec.NewSteps(st, hooks), cfg, deployexec.Chrome())
	sess := slot.take()
	if sess != nil {
		defer sess.close()
	}
	restoreLogs()
	recordDeployHistory(st, opts.ProjectRoot)
	if sess != nil && sess.state.Started {
		printDeployRecap(out, st)
		if err != nil {
			return errors.Join(err, reportLifecycleOutcome(cmd, result, sess.state))
		}
		return reportLifecycleOutcome(cmd, result, sess.state)
	}
	if err != nil {
		// A tea failure mid-install must still surface the resume marker, not
		// read as a configuration problem.
		if st.Started && !st.Executed {
			return &errtypes.ClusterError{Msg: deployInterruptedMsg, Err: err}
		}
		return (&errtypes.ConfigError{Msg: "deploy wizard failed", Err: err}).
			WithHint("try again, or re-run with --no-tui for the plain checklist")
	}
	return reportDeployStreamOutcome(out, st)
}

func deployConsoleURL(cfg *config.Config) string {
	for _, fact := range render.NewPostDeployFacts(cfg, nil, nil).Access {
		if fact.Key == "console" {
			return fact.Link
		}
	}
	return ""
}

func deployFollowOnHooks(cmd *cobra.Command, cfg *config.Config, slot *lifecycleSlot) deployexec.Hooks {
	flows := hubFlows(cmd, cfg, slot)
	hooks := deployexec.Hooks{
		Finish: &deployexec.FinishHooks{
			ManageNodes: deployexec.NextFlow(flows.ManageNodes),
		},
	}
	if url := deployConsoleURL(cfg); url != "" {
		hooks.Finish.OpenConsole = func() tea.Cmd { return openConsole(cmd.Context(), url) }
	}
	return hooks
}

func openConsole(ctx context.Context, url string) tea.Cmd {
	name, args, err := browserCommand(runtime.GOOS, url)
	if err != nil {
		return func() tea.Msg { return wizard.ErrorSetMsg{Error: err} }
	}
	return tea.ExecProcess(exec.CommandContext(ctx, name, args...), func(err error) tea.Msg {
		if err == nil {
			return nil
		}
		return wizard.ErrorSetMsg{Error: fmt.Errorf("open console: %w", err)}
	})
}

func browserCommand(goos, url string) (name string, args []string, err error) {
	switch goos {
	case "darwin", "dragonfly", "freebsd", "netbsd", "openbsd":
		return "open", []string{url}, nil
	case "android", "illumos", "linux", "solaris":
		return "xdg-open", []string{url}, nil
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}, nil
	default:
		return "", nil, fmt.Errorf("open console: unsupported on %s", goos)
	}
}

// loadDeployHistory seeds the stream screen's weight model and ETA from
// the persisted per-step durations beside the deploy-state marker; the
// scripted demo feed renders without one.
func loadDeployHistory(st *deployexec.State, projectRoot string) {
	if os.Getenv(wizardDemoEnv) != "" {
		return
	}
	st.History = deployexec.LoadStepHistory(workspace.WorkDir(projectRoot), st.Cfg.Cluster.Name)
}

// recordDeployHistory folds the run's measured step durations back into the
// history store — failures included, since their finished steps carry real
// measurements; a demo run must not write schedule data into the project.
// Executed gates the read: it is set only after the final event was consumed
// (a channel-synchronized handoff ordered after the engine's Steps write),
// while a force-quit-abandoned engine goroutine may still be writing Steps
// when RunFlow returns.
func recordDeployHistory(st *deployexec.State, projectRoot string) {
	if os.Getenv(wizardDemoEnv) != "" || !st.Executed || len(st.Steps) == 0 {
		return
	}
	if err := deployexec.RecordStepHistory(workspace.WorkDir(projectRoot), st.RunID, st.Cfg.Cluster.Name, st.Steps); err != nil {
		logutil.Warn("could not record step duration history", logutil.LF("err", err))
	}
}

// deployStreamSession assembles the screen's hooks and wires the cancel its
// ctrl+c guard calls. A feed that brings its own cancel keeps it: the demo's
// events come from its own context, so the engine's cancel would leave it
// running.
func deployStreamSession(streamCtx context.Context, cancel context.CancelFunc, cfg *config.Config, opts *deploy.Options) deployexec.Hooks {
	hooks := deployStreamHooks(streamCtx, cfg, opts)
	if hooks.CancelDeploy == nil {
		hooks.CancelDeploy = cancel
	}
	if hooks.Done == nil {
		hooks.Done = streamCtx.Done()
	}
	return hooks
}

// deployStreamHooks builds the stream screen's engine hook: the real deploy
// engine, or deployexec.DemoHooks' scripted feed under OKDCTL_WIZARD_DEMO.
func deployStreamHooks(streamCtx context.Context, cfg *config.Config, opts *deploy.Options) deployexec.Hooks {
	if os.Getenv(wizardDemoEnv) != "" {
		return deployexec.DemoHooks(demoStreamStepDelay)
	}
	return deployexec.Hooks{
		Execute: func(st *deployexec.State, events chan<- deployexec.Event) error {
			// A copy, so arming the reporter never reaches back into the
			// caller's own options.
			run := *opts
			run.Reporter = deployexec.NewRecorder(streamCtx, events)
			// The screen owns the terminal, so the engine must not tee raw
			// subprocess output onto it; the sink still keeps every byte.
			run.Verbose = false
			// io.Discard, not the command's stdout: Options.Reporter already
			// suppresses every box Execute would write, and the screen owns the
			// terminal while it runs.
			outcome, err := deployExecuteFn(streamCtx, cfg, &run, io.Discard)
			if outcome != nil {
				st.Summary, st.Steps, st.RunID = outcome.Result, outcome.Steps, outcome.RunID
				// The engine's own span excludes the run lock and config
				// validation the screen was already up for.
				st.Elapsed = outcome.Duration
			}
			return err
		},
	}
}

// reportDeployStreamOutcome maps the stream screen's terminal state to a
// truthful exit; an interrupted mid-install run exits non-zero instead of
// claiming a deployed cluster.
func reportDeployStreamOutcome(out io.Writer, st *deployexec.State) error {
	switch {
	case st.Started && !st.Executed:
		return &errtypes.ClusterError{Msg: deployInterruptedMsg}
	case errors.Is(st.Result, context.Canceled):
		// A graceful cancel unwound the engine cleanly, so it returns only
		// context.Canceled — on its own that reads as a bare failure and drops
		// the resume guidance the plain checklist prints.
		return &errtypes.ClusterError{Msg: deployInterruptedMsg}
	case st.Executed && st.Result != nil:
		return st.Result
	case st.Executed:
		printDeployRecap(out, st)
		return nil
	default:
		logutil.Info("no changes made")
		return nil
	}
}

// printDeployRecap prints the short plain-text recap of a finished deploy: the
// wizard's AltScreen already cleared the summary box from scrollback on exit,
// leaving no durable record, so this reprints the access lines plus the
// operator's next step — never the box itself.
func printDeployRecap(out io.Writer, st *deployexec.State) {
	if st.Cfg == nil {
		return
	}
	for _, line := range render.PostDeployRecapLines(st.Cfg, st.RunID, st.Elapsed) {
		fmt.Fprintln(out, line)
	}
}
