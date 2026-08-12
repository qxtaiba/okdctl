package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/clusterstatus"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/lifecycle"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

var nodeManageCmd = &cobra.Command{
	Use:   "manage",
	Short: "Interactively manage node lifecycle (resize / add / remove)",
	Long: `Launch the Cluster Lifecycle wizard: pick an operation, pick a target from
the live node list, enter parameters, review a real dry-run plan of the
exact blast radius, then execute with the same guards and health gates as
the flag-driven node verbs.

Requires a terminal and an existing configuration; use 'okdctl node
resize/add/remove' for automation.`,
	Example: `  okdctl node manage`,
	Args:    cobra.NoArgs,
	RunE:    runNodeManage,
}

func runNodeManage(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	if !term.IsTerminal(int(os.Stdout.Fd())) || !term.IsTerminal(int(os.Stdin.Fd())) {
		return &errtypes.UsageError{Msg: "node manage needs a terminal; use 'okdctl node resize/add/remove' for automation"}
	}

	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	// The wizard owns the terminal from here on; no line-owner spinner or
	// download progress bar may render beneath the AltScreen.
	logutil.SetProgressBarsEnabled(false)

	env, err := prepareNodeOpsEnv(ctx, cfg, true)
	if err != nil {
		return err
	}
	defer env.close()

	cl, err := clusterstatus.NewClient(env.projectRoot)
	if err != nil {
		return err
	}

	marker, err := node.ReadOpMarker(workspace.WorkDir(env.projectRoot), cfg.Cluster.Name)
	if err != nil {
		return err
	}

	// opCtx is cancelled by the execution screen's graceful-cancel path
	// (first ctrl+c); the backend unwinds and leaves its resume marker.
	opCtx, cancelOp := context.WithCancel(ctx)
	defer cancelOp()

	st := &lifecycle.State{Cfg: cfg, Marker: marker}
	hooks := lifecycle.Hooks{
		ListNodes: func() ([]cluster.NodeDetail, error) { return cl.ListNodes(ctx) },
		DryRun: func(s *lifecycle.State) (*node.OpPlan, error) {
			rc, err := env.newRunner(cmd, cfg, "manage", nodeConsent{dryRun: true}, fileOnlySlog(), subprocSink())
			if err != nil {
				return nil, err
			}
			defer rc.cleanup()
			var captured *node.OpPlan
			rc.runner.Preview = func(p *node.OpPlan) { captured = p }
			if err := runLifecycleOp(ctx, rc, s); err != nil {
				return nil, err
			}
			return captured, nil
		},
		CancelOp: cancelOp,
		Execute: func(s *lifecycle.State, events chan<- lifecycle.ExecEvent) error {
			return executeLifecycleOp(opCtx, cmd, cfg, env, s, events)
		},
	}

	result, err := wizard.RunFlow(ctx, lifecycle.NewSteps(st, hooks), cfg, lifecycleChrome())
	if err != nil {
		// A tea failure mid-execution must still surface the resume marker,
		// not read as a configuration problem.
		if st.Started && !st.Executed {
			return &errtypes.ClusterError{Msg: "execution was interrupted mid-operation; the op marker records the in-flight step — re-run 'okdctl node manage' (or the matching node verb) to resume", Err: err}
		}
		return &errtypes.ConfigError{Msg: "lifecycle wizard", Err: err}
	}
	return reportLifecycleOutcome(cmd, result, st)
}

// reportLifecycleOutcome maps the wizard's terminal state to a truthful
// exit: "no changes made" is only ever printed when execution never
// started; a run interrupted mid-execution surfaces the resume marker and
// exits non-zero instead of claiming a clean state.
func reportLifecycleOutcome(cmd *cobra.Command, result wizard.Result, st *lifecycle.State) error {
	switch {
	case st.Started && !st.Executed:
		return &errtypes.ClusterError{Msg: "execution was interrupted mid-operation; the op marker records the in-flight step — re-run 'okdctl node manage' (or the matching node verb) to resume"}
	case st.Executed && st.Result != nil:
		return st.Result
	case st.Executed:
		fmt.Fprint(cmd.OutOrStdout(), render.NodeOpComplete(st.Plan, st.Elapsed))
		return nil
	case result.Cancelled || !st.Proceed:
		logutil.Info("no changes made")
		return nil
	default:
		return nil
	}
}

// executeLifecycleOp runs the wizard-approved operation inside the TUI's
// AltScreen: progress flows over the event channel (Reporter spans +
// OnStep transitions) and slog goes to the okdctl.log sink only. Consent
// was granted on the preview/confirm screens, so the runner's ConfirmFunc
// only cross-checks that the world still matches the approved plan.
func executeLifecycleOp(opCtx context.Context, cmd *cobra.Command, cfg *config.Config, env *nodeOpsEnv, st *lifecycle.State, events chan<- lifecycle.ExecEvent) error {
	rc, err := env.newRunner(cmd, cfg, "manage", nodeConsent{}, fileOnlySlog(), subprocSink())
	if err != nil {
		return err
	}
	defer rc.cleanup()

	approved := st.Plan
	rc.runner.Confirm = func(_ context.Context, p *node.OpPlan) (bool, error) {
		return lifecycle.PlansEquivalent(approved, p), nil
	}
	rc.runner.Reporter = func(desc string) func() {
		start := time.Now()
		events <- lifecycle.ExecEvent{Desc: desc}
		return func() { events <- lifecycle.ExecEvent{Desc: desc, Done: true, Took: time.Since(start)} }
	}
	rc.runner.OnStep = func(target string, step node.Step) {
		events <- lifecycle.ExecEvent{Node: target, Step: step}
	}
	if err := runLifecycleOp(opCtx, rc, st); err != nil {
		if errors.Is(err, node.ErrDeclined) {
			return &errtypes.ClusterError{Msg: "the cluster changed since the preview — re-run 'okdctl node manage' to re-plan"}
		}
		return err
	}
	return nil
}

// subprocSink is where the setup executor's subprocess streams (scp, ISO
// tooling) go while the wizard owns the terminal: the okdctl.log sink, or
// discard when no sink opened.
func subprocSink() io.Writer {
	if runLogSink == nil {
		return io.Discard
	}
	return runLogSink
}

// fileOnlySlog returns a redact-wrapped slog writing only to the okdctl.log
// sink — never stderr, which the AltScreen wizard owns during execution.
func fileOnlySlog() *slog.Logger {
	if runLogSink == nil {
		return logutil.NopLogger
	}
	return slog.New(logutil.NewRedactHandler(slog.NewTextHandler(runLogSink, nil)))
}

// runLifecycleOp dispatches the wizard-collected operation onto the
// runner, merging the host-probe budget the same way the flag verbs do.
func runLifecycleOp(ctx context.Context, rc *nodeRunnerCtx, st *lifecycle.State) error {
	switch st.Op {
	case node.OpResize:
		return rc.runner.Resize(ctx, st.Scope, resizeOptsFromWizard(rc, st))
	case node.OpAdd:
		return rc.runner.AddWorkers(ctx, addOptsFromWizard(rc, st))
	case node.OpRemove:
		return rc.runner.RemoveWorker(ctx, st.Target, lifecycle.RemoveOptionsFrom(st))
	default:
		return &errtypes.UsageError{Msg: fmt.Sprintf("unsupported lifecycle op %q", st.Op)}
	}
}

// resizeOptsFromWizard merges the wizard-collected resize dimensions with the
// read-only Proxmox probe results the flag verb (runNodeResize) also feeds
// Resize, so the memory and datastore guards are armed the same way
// regardless of entry point. A dropped merge here would leave the guard it
// backs disarmed for every TUI-driven resize while the flag verb stayed
// protected.
func resizeOptsFromWizard(rc *nodeRunnerCtx, st *lifecycle.State) node.ResizeOptions {
	opts := lifecycle.ResizeOptionsFrom(st)
	opts.HostTotalMiB, opts.HostAllocatedMiB = rc.HostTotalMiB, rc.HostAllocatedMiB
	opts.DatastoreAvailGB = rc.DatastoreAvailGB
	return opts
}

// addOptsFromWizard mirrors resizeOptsFromWizard for node add, which only
// carries the memory-budget probe (add never resizes a disk).
func addOptsFromWizard(rc *nodeRunnerCtx, st *lifecycle.State) node.AddOptions {
	opts := lifecycle.AddOptionsFrom(st)
	opts.HostTotalMiB, opts.HostAllocatedMiB = rc.HostTotalMiB, rc.HostAllocatedMiB
	return opts
}

// lifecycleChrome keeps the shared brand tagline but swaps the context
// badge to the cluster name — the lifecycle flow operates an existing
// cluster rather than assembling a distribution choice.
func lifecycleChrome() wizard.FlowChrome {
	return wizard.FlowChrome{
		Tagline: "okd over proxmox, the easy way",
		Badge:   func(c *config.Config) string { return c.Cluster.Name },
	}
}
