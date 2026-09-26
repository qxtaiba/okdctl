package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/clusterstatus"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/tui/logview"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/lifecycle"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

// lifecycleInterruptedMsg is shared so runNodeManage's tea-failure path and
// reportLifecycleOutcome present identical guidance.
const lifecycleInterruptedMsg = "execution was interrupted mid-operation; the op marker records the in-flight step — re-run 'okdctl node manage' (or the matching node verb) to resume"

var nodeManageCmd = &cobra.Command{
	Use:   "manage",
	Short: "Interactively manage node lifecycle (resize / add / remove)",
	Long: `Launch the Cluster Lifecycle flow: pick an operation, pick a target from
the live node list, enter parameters, review a real dry-run plan of the
exact blast radius, then execute with the same guards and health gates as
the flag-driven node verbs.

Use --accessible or OKDCTL_ACCESSIBLE=1 for sequential plain-text prompts.

Requires a terminal and an existing configuration; use 'okdctl node
resize/add/remove' for automation.`,
	Example: `  okdctl node manage
  okdctl node manage --accessible`,
	Args: cobra.NoArgs,
	RunE: runNodeManage,
}

var nodeManageAccessible bool

func init() {
	nodeManageCmd.Flags().BoolVar(&nodeManageAccessible, "accessible", false, "use sequential plain-text prompts instead of the full-screen wizard")
}

func runNodeManage(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	if !term.IsTerminal(int(os.Stdout.Fd())) || !term.IsTerminal(int(os.Stdin.Fd())) {
		return &errtypes.UsageError{Msg: "node manage needs a terminal; use 'okdctl node resize/add/remove' for automation"}
	}

	cfg, err := lifecycleConfig()
	if err != nil {
		return err
	}

	// The wizard owns the terminal from here on; no spinner/progress bar may
	// render beneath the AltScreen.
	logutil.SetProgressBarsEnabled(false)

	sess, err := newLifecycleSession(cmd, cfg)
	if err != nil {
		return err
	}
	defer sess.close()
	if accessibleRequested(nodeManageAccessible, os.Getenv) {
		return runAccessibleNodeManageWith(ctx, newTerminalAccessiblePrompt(cmd.InOrStdin(), cmd.OutOrStdout(), int(os.Stdin.Fd())), sess)
	}

	result, err := wizard.RunFlow(ctx, sess.steps, cfg, lifecycle.Chrome())
	if err != nil {
		// A tea failure mid-execution must still surface the resume marker, not
		// read as a configuration problem.
		if sess.state.Started && !sess.state.Executed {
			return &errtypes.ClusterError{Msg: lifecycleInterruptedMsg, Err: err}
		}
		return (&errtypes.ConfigError{Msg: "lifecycle wizard failed", Err: err}).
			WithHint("try again, or use 'okdctl node resize/add/remove' instead")
	}
	return reportLifecycleOutcome(cmd, result, sess.state)
}

func runAccessibleNodeManageWith(ctx context.Context, prompt accessiblePrompt, sess *lifecycleSession) error {
	st := sess.state
	if st == nil || st.Cfg == nil {
		return errors.New("node manage: missing lifecycle state")
	}
	if err := accessibleNodeOperation(ctx, prompt, st); err != nil {
		return accessibleNodeFinishBeforeRun(prompt, err)
	}
	if !st.Resume {
		if st.Op != node.OpAdd {
			if err := accessibleNodeTarget(ctx, prompt, st, sess.hooks); err != nil {
				return accessibleNodeFinishBeforeRun(prompt, err)
			}
		}
	}
	if err := accessibleNodeParams(ctx, prompt, st); err != nil {
		return accessibleNodeFinishBeforeRun(prompt, err)
	}
	if sess.hooks.DryRun == nil {
		return errors.New("node manage: dry-run hook is unavailable")
	}
	plan, err := sess.hooks.DryRun(st)
	if err != nil {
		return err
	}
	if plan == nil {
		return errors.New("node manage: dry-run returned no plan")
	}
	st.Plan = plan
	fmt.Fprintln(prompt.writer(), "\nPlan review")
	fmt.Fprintln(prompt.writer(), render.NodeOpConfirm(plan))
	if plan.DestroysData() {
		name, err := accessibleNodeLine(ctx, prompt, "type cluster name to confirm destruction", "")
		if err != nil {
			return accessibleNodeFinishBeforeRun(prompt, err)
		}
		if name != st.Cfg.Cluster.Name {
			return &errtypes.UsageError{Msg: "cluster name did not match; operation was not executed"}
		}
	}
	answer, err := accessibleNodeLine(ctx, prompt, "execute this plan? (y/N)", "no")
	if err != nil {
		return accessibleNodeFinishBeforeRun(prompt, err)
	}
	switch strings.ToLower(answer) {
	case "y", accessibleYesToken:
	case "", "n", "no":
		fmt.Fprintln(prompt.writer(), "No changes made.")
		return nil
	default:
		return &errtypes.UsageError{Msg: "answer yes or no; no changes were made"}
	}
	if sess.hooks.Execute == nil {
		return errors.New("node manage: execute hook is unavailable")
	}
	st.Proceed = true
	st.Started = true
	fmt.Fprintln(prompt.writer(), "Executing the reviewed plan.")
	events := make(chan lifecycle.ExecEvent, 64)
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		for ev := range events {
			if ev.Node != "" && ev.Step != "" {
				fmt.Fprintf(prompt.writer(), "  %s: %s\n", ev.Node, ev.Step)
			}
		}
	}()
	started := time.Now()
	err = sess.hooks.Execute(st, events)
	close(events)
	<-drained
	st.Elapsed = time.Since(started)
	st.Result = err
	st.Executed = true
	cmd := &cobra.Command{}
	cmd.SetOut(prompt.writer())
	return reportLifecycleOutcome(cmd, wizard.Result{Completed: true}, st)
}

func accessibleNodeOperation(ctx context.Context, prompt accessiblePrompt, st *lifecycle.State) error {
	current := "cancel"
	if st.Marker != nil {
		current = "resume"
		fmt.Fprintf(prompt.writer(), "Interrupted %s at %s for %s. Choosing another operation acknowledges that marker.\n",
			st.Marker.Op, st.Marker.Step, st.Marker.Target)
	}
	value, err := accessibleNodeLine(ctx, prompt, "operation (resume/resize/add/remove; blank cancels)", current)
	if err != nil {
		return err
	}
	var op node.Op
	resume := false
	switch strings.ToLower(value) {
	case "resume":
		if st.Marker == nil {
			return &errtypes.UsageError{Msg: "there is no interrupted node operation to resume"}
		}
		op, resume = st.Marker.Op, true
	case "resize":
		op = node.OpResize
	case "add":
		op = node.OpAdd
	case "remove":
		op = node.OpRemove
	case "", "cancel":
		return errAccessibleNodeCancelled
	default:
		return &errtypes.UsageError{Msg: "choose resume, resize, add, remove, or cancel"}
	}
	st.Op = op
	st.Resume = resume
	st.Ack = st.Marker != nil && !resume
	st.Scope = node.ResizeScope{}
	st.Target = ""
	if resume {
		switch op {
		case node.OpResize:
			st.Scope = node.ResizeScope{Node: st.Marker.Target}
		case node.OpRemove:
			st.Target = st.Marker.Target
		}
	}
	return nil
}

func accessibleNodeTarget(ctx context.Context, prompt accessiblePrompt, st *lifecycle.State, hooks lifecycle.Hooks) error {
	if hooks.ListNodes == nil {
		return errors.New("node manage: node-list hook is unavailable")
	}
	nodes, err := hooks.ListNodes()
	if err != nil {
		return err
	}
	st.Nodes = nodes
	if st.Op == node.OpRemove {
		workers := make([]cluster.NodeDetail, 0, len(nodes))
		for i := range nodes {
			if nodes[i].Role == nodetypes.RoleWorker {
				workers = append(workers, nodes[i])
			}
		}
		sort.SliceStable(workers, func(i, j int) bool {
			a, aok := cluster.NodeIndex(workers[i].Name)
			b, bok := cluster.NodeIndex(workers[j].Name)
			if !aok || !bok {
				return aok
			}
			return a > b
		})
		if len(workers) == 0 {
			return &errtypes.UsageError{Msg: "no eligible worker nodes found for removal"}
		}
		fmt.Fprintf(prompt.writer(), "Only the highest-numbered worker is eligible: %s\n", workers[0].Name)
		value, err := accessibleNodeLine(ctx, prompt, "worker to remove (highest-numbered worker only)", workers[0].Name)
		if err != nil {
			return err
		}
		if value != workers[0].Name {
			return &errtypes.UsageError{Msg: "choose the highest-numbered worker shown; no changes were made"}
		}
		st.Target = value
		return nil
	}
	fmt.Fprintln(prompt.writer(), "Eligible targets:")
	for i := range nodes {
		fmt.Fprintf(prompt.writer(), "  %s (%s)\n", nodes[i].Name, nodes[i].Role)
	}
	value, err := accessibleNodeLine(ctx, prompt, "target (masters, workers, or node name)", "")
	if err != nil {
		return err
	}
	switch value {
	case "masters":
		st.Scope = node.ResizeScope{Role: nodetypes.RoleMaster}
	case "workers":
		st.Scope = node.ResizeScope{Role: nodetypes.RoleWorker}
	default:
		for i := range nodes {
			if value == nodes[i].Name {
				st.Scope = node.ResizeScope{Node: value}
				return nil
			}
		}
		return &errtypes.UsageError{Msg: "choose masters, workers, or a listed node name"}
	}
	return nil
}

const accessibleYesToken = "yes"

func accessibleNodeParams(ctx context.Context, prompt accessiblePrompt, st *lifecycle.State) error {
	switch st.Op {
	case node.OpAdd:
		return accessibleNodeAddParams(ctx, prompt, st)
	case node.OpRemove:
		return accessibleNodeRemoveParams(ctx, prompt, st)
	case node.OpResize:
		return accessibleNodeResizeParams(ctx, prompt, st)
	default:
		return &errtypes.UsageError{Msg: "unsupported lifecycle operation"}
	}
}

func accessibleNodeAddParams(ctx context.Context, prompt accessiblePrompt, st *lifecycle.State) error {
	value, err := accessibleNodeLine(ctx, prompt, "workers to add", "1")
	if err != nil {
		return err
	}
	st.Count, err = strconv.Atoi(value)
	if err != nil || st.Count < 1 {
		return &errtypes.UsageError{Msg: "workers to add must be a whole number >= 1"}
	}
	return nil
}

func accessibleNodeRemoveParams(ctx context.Context, prompt accessiblePrompt, st *lifecycle.State) error {
	if err := accessibleNodeDrainParams(ctx, prompt, st); err != nil {
		return err
	}
	value, err := accessibleNodeLine(ctx, prompt, "force removal with storage data loss? (y/N)", "no")
	if err != nil {
		return err
	}
	st.ForceStorage, err = parseAccessibleYesNo(value)
	if err != nil {
		return &errtypes.UsageError{Msg: "answer yes or no for the storage data-loss override"}
	}
	return nil
}

func accessibleNodeResizeParams(ctx context.Context, prompt accessiblePrompt, st *lifecycle.State) error {
	role := st.Scope.Role
	if role == "" {
		for i := range st.Nodes {
			if st.Nodes[i].Name == st.Scope.Node {
				role = st.Nodes[i].Role
			}
		}
		if role == "" && strings.Contains(st.Scope.Node, "master") {
			role = nodetypes.RoleMaster
		}
	}
	current := st.Cfg.Topology.Workers
	if role == nodetypes.RoleMaster {
		current = st.Cfg.Topology.ControlPlane
	}
	fields := []struct {
		label string
		value int
		set   func(int)
	}{
		{"memory (mb; 0 keeps current)", current.MemoryMB, func(v int) { st.MemoryMB = v }},
		{"vcpus (0 keeps current)", 0, func(v int) { st.CPU = v }},
		{"os disk (gb; 0 keeps current, grow-only)", 0, func(v int) { st.OSDiskGB = v }},
	}
	for _, field := range fields {
		value, err := accessibleNodeLine(ctx, prompt, field.label, strconv.Itoa(field.value))
		if err != nil {
			return err
		}
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return &errtypes.UsageError{Msg: field.label + " must be a whole number >= 0"}
		}
		if strings.HasPrefix(field.label, "memory") && n > 0 && n < 8192 {
			return &errtypes.UsageError{Msg: "memory must be at least 8192 MB, or 0 to keep current"}
		}
		field.set(n)
	}
	if st.MemoryMB == 0 && st.CPU == 0 && st.OSDiskGB == 0 {
		return &errtypes.UsageError{Msg: "resize requires at least one of memory, vcpus, or os disk"}
	}
	if err := validateAccessibleDiskResize(st); err != nil {
		return err
	}
	return accessibleNodeDrainParams(ctx, prompt, st)
}

func validateAccessibleDiskResize(st *lifecycle.State) error {
	if st.OSDiskGB == 0 {
		return nil
	}
	if st.Scope.Node != "" {
		return &errtypes.UsageError{Msg: "os disk is role-scoped; choose masters or workers"}
	}
	current := st.Cfg.Topology.Workers.DiskGB
	if st.Scope.Role == nodetypes.RoleMaster {
		current = st.Cfg.Topology.ControlPlane.DiskGB
	}
	if st.OSDiskGB <= current {
		return &errtypes.UsageError{Msg: fmt.Sprintf("os disk size must exceed the current %d GiB", current)}
	}
	return nil
}

func accessibleNodeDrainParams(ctx context.Context, prompt accessiblePrompt, st *lifecycle.State) error {
	mode, err := accessibleNodeLine(ctx, prompt, "drain mode (drain/skip)", "drain")
	if err != nil {
		return err
	}
	switch strings.ToLower(mode) {
	case "drain":
		st.SkipDrain = false
	case "skip":
		st.SkipDrain = true
	default:
		return &errtypes.UsageError{Msg: "drain mode must be drain or skip"}
	}
	timeout, err := accessibleNodeLine(ctx, prompt, "drain timeout", "10m")
	if err != nil {
		return err
	}
	parsed, err := time.ParseDuration(timeout)
	if err != nil || parsed <= 0 {
		return &errtypes.UsageError{Msg: "drain timeout must be a positive duration like 10m or 1h"}
	}
	st.DrainTimeout = timeout
	return nil
}

func accessibleNodeLine(ctx context.Context, prompt accessiblePrompt, label, current string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", errAccessibleNodeCancelled
	}
	value, err := prompt.line(label, current)
	if errors.Is(err, io.EOF) || ctx.Err() != nil {
		return "", errAccessibleNodeCancelled
	}
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(value) == "" {
		return strings.TrimSpace(current), nil
	}
	return strings.TrimSpace(value), nil
}

func accessibleNodeFinishBeforeRun(prompt accessiblePrompt, err error) error {
	if errors.Is(err, errAccessibleNodeCancelled) {
		fmt.Fprintln(prompt.writer(), "No changes made.")
		return nil
	}
	return err
}

var errAccessibleNodeCancelled = errors.New("accessible node operation cancelled")

func parseAccessibleYesNo(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "n", "no":
		return false, nil
	case "y", accessibleYesToken:
		return true, nil
	default:
		return false, errors.New("expected yes or no")
	}
}

// lifecycleConfig resolves the config the Cluster Lifecycle flow runs against:
// the static demo identity under OKDCTL_WIZARD_DEMO, whose cluster name matches
// lifecycle.DemoHooks' fixture, or the saved configuration otherwise.
func lifecycleConfig() (*config.Config, error) {
	if os.Getenv(wizardDemoEnv) != "" {
		return demoConfig(), nil
	}
	return loadConfig(cfgFile)
}

// lifecycleSession is an assembled Cluster Lifecycle flow: its steps, the state
// they write into, and the teardown its environment needs.
type lifecycleSession struct {
	steps []wizard.WizardStep
	state *lifecycle.State
	hooks lifecycle.Hooks
	// close zeroizes credentials and cancels the op context; it must run only
	// after the wizard exits, since the hooks the steps call hold both.
	close func()
}

// newLifecycleSession assembles the Cluster Lifecycle flow against cfg, driven
// by the same hooks okdctl node manage builds — or lifecycle.DemoHooks' static
// six-node fixture under OKDCTL_WIZARD_DEMO. Shared with the hero-hub's
// manage-nodes verb, which swaps this flow in mid-session, so the two entry
// points can never drift into different guards.
func newLifecycleSession(cmd *cobra.Command, cfg *config.Config) (*lifecycleSession, error) {
	if os.Getenv(wizardDemoEnv) != "" {
		st := &lifecycle.State{Cfg: cfg}
		hooks := lifecycle.DemoHooks(demoExecStepDelay)
		return &lifecycleSession{
			steps: lifecycle.NewSteps(st, hooks),
			state: st,
			hooks: hooks,
			close: func() {},
		}, nil
	}

	ctx := cmd.Context()
	env, err := prepareNodeOpsEnv(ctx, cfg, true)
	if err != nil {
		return nil, err
	}

	cl, err := clusterstatus.NewClient(env.projectRoot)
	if err != nil {
		env.close()
		return nil, err
	}

	marker, err := node.ReadOpMarker(workspace.WorkDir(env.projectRoot), cfg.Cluster.Name)
	if err != nil {
		env.close()
		return nil, err
	}

	// opCtx is cancelled by the execution screen's graceful-cancel path (first
	// ctrl+c); the backend unwinds and leaves its resume marker.
	opCtx, cancelOp := context.WithCancel(ctx)

	// The ring tees the human log stream into the exec screen's log surface
	// on its way to the run log, so terraform applies, drains, and
	// power-cycles stream onto the screen instead of running blind.
	ring := logview.NewRing(logview.DefaultCap)
	lg := ringSlog(ring)

	st := &lifecycle.State{Cfg: cfg, Marker: marker}
	hooks := lifecycle.Hooks{
		ListNodes: func() ([]cluster.NodeDetail, error) { return cl.ListNodes(ctx) },
		DryRun: func(s *lifecycle.State) (*node.OpPlan, error) {
			rc, err := env.newRunner(cmd, cfg, "manage", nodeConsent{dryRun: true}, lg, subprocSink())
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
		Logs:     ring,
		LogPath:  runLogPath,
		Done:     opCtx.Done(),
		Execute: func(s *lifecycle.State, events chan<- lifecycle.ExecEvent) error {
			return executeLifecycleOp(opCtx, cmd, cfg, env, s, events, lg)
		},
	}

	return &lifecycleSession{
		steps: lifecycle.NewSteps(st, hooks),
		state: st,
		hooks: hooks,
		close: func() {
			cancelOp()
			env.close()
		},
	}, nil
}

// reportLifecycleOutcome maps wizard terminal state to a truthful exit; an
// interrupted mid-execution run exits non-zero instead of claiming a clean
// state.
func reportLifecycleOutcome(cmd *cobra.Command, result wizard.Result, st *lifecycle.State) error {
	switch {
	case st.Started && !st.Executed:
		return &errtypes.ClusterError{Msg: lifecycleInterruptedMsg}
	case st.Executed && st.Result != nil:
		return st.Result
	case st.Executed:
		printLifecycleRecap(cmd, st)
		return nil
	case result.Cancelled || !st.Proceed:
		logutil.Info("no changes made")
		return nil
	default:
		return nil
	}
}

// printLifecycleRecap prints a short plain-text recap of the finished op:
// the wizard's AltScreen already cleared the done card from scrollback on
// exit, leaving no durable record of what happened, so this reprints a
// one-line summary plus the operator's next-step commands (RULING —
// reversing the earlier prints-nothing-on-success ruling; does NOT reprint
// the box itself, that reversal was ruled in the refit and stands). Gated
// on the caller: runNodeManage already refuses to start without a TTY, so
// this print is TTY-gated transitively rather than re-checking here.
func printLifecycleRecap(cmd *cobra.Command, st *lifecycle.State) {
	if st.Plan == nil {
		return
	}
	for _, line := range render.NodeOpRecapLines(st.Plan, st.Elapsed) {
		fmt.Fprintln(cmd.OutOrStdout(), line)
	}
}

// executeLifecycleOp runs the wizard-approved op inside the AltScreen;
// ConfirmFunc only cross-checks the world still matches the plan already
// approved on the preview screen.
func executeLifecycleOp(opCtx context.Context, cmd *cobra.Command, cfg *config.Config, env *nodeOpsEnv, st *lifecycle.State, events chan<- lifecycle.ExecEvent, lg *slog.Logger) error {
	rc, err := env.newRunner(cmd, cfg, "manage", nodeConsent{}, lg, subprocSink())
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
		sendExecEvent(opCtx, events, &lifecycle.ExecEvent{Desc: desc})
		return func() {
			sendExecEvent(opCtx, events, &lifecycle.ExecEvent{Desc: desc, Done: true, Took: time.Since(start)})
		}
	}
	rc.runner.OnStep = func(target string, step node.Step) {
		sendExecEvent(opCtx, events, &lifecycle.ExecEvent{Node: target, Step: step})
	}
	if err := runLifecycleOp(opCtx, rc, st); err != nil {
		if errors.Is(err, node.ErrDeclined) {
			return &errtypes.ClusterError{Msg: "the cluster changed since the preview — re-run 'okdctl node manage' to re-plan"}
		}
		return err
	}
	return nil
}

// subprocSink routes subprocess streams to the okdctl.log sink while the wizard
// owns the terminal, or discards when no sink is open.
func subprocSink() io.Writer {
	if runLogSink == nil {
		return io.Discard
	}
	return runLogSink
}

// ringSlog tees the session's log stream into the exec screen's log ring on
// its way to the okdctl.log sink, never stderr, which the AltScreen wizard
// owns during execution; redaction wraps the tee, so the on-screen pane
// only ever sees scrubbed records.
func ringSlog(ring *logview.Ring) *slog.Logger {
	var next slog.Handler
	if runLogSink != nil {
		next = slog.NewTextHandler(runLogSink, nil)
	}
	return slog.New(logutil.NewRedactHandler(ring.Handler(next)))
}

// sendExecEvent delivers ev, abandoning it only once the op's context is
// gone AND the feed cannot accept it — a chatty unwind after a force-quit
// must never strand the runner goroutine (holding the run lock and a
// terraform subprocess) on a feed nobody drains. Delivery is biased: the
// graceful cancel cancels this very context, and a uniform select would
// drop events the exec screen is still draining.
func sendExecEvent(ctx context.Context, events chan<- lifecycle.ExecEvent, ev *lifecycle.ExecEvent) {
	select {
	case events <- *ev:
	default:
		select {
		case <-ctx.Done():
		case events <- *ev:
		}
	}
}

// runLifecycleOp dispatches the wizard-collected op onto the runner, merging
// the host-probe budget the same way the flag verbs do.
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

// resizeOptsFromWizard arms the memory and datastore guards for TUI-driven
// resizes with the same probe results runNodeResize feeds the flag verb; a
// dropped merge here disarms them for every wizard resize.
func resizeOptsFromWizard(rc *nodeRunnerCtx, st *lifecycle.State) node.ResizeOptions {
	opts := lifecycle.ResizeOptionsFrom(st)
	opts.HostTotalMiB, opts.HostAllocatedMiB = rc.HostTotalMiB, rc.HostAllocatedMiB
	opts.DatastoreAvailGB = rc.DatastoreAvailGB
	return opts
}

// addOptsFromWizard mirrors resizeOptsFromWizard for node add, which carries
// only the memory-budget probe.
func addOptsFromWizard(rc *nodeRunnerCtx, st *lifecycle.State) node.AddOptions {
	opts := lifecycle.AddOptionsFrom(st)
	opts.HostTotalMiB, opts.HostAllocatedMiB = rc.HostTotalMiB, rc.HostAllocatedMiB
	return opts
}

// lifecycleSlot hands the manage-nodes session between the goroutine that
// builds it and the main path that closes it, so whichever of the two arrives
// second — a raced quit or the finished probe — owns the one close.
type lifecycleSlot struct {
	mu      sync.Mutex
	session *lifecycleSession
	taken   bool
}

// put registers sess as the session to close, or closes it immediately itself
// when take has already run, reporting whether it was accepted.
func (s *lifecycleSlot) put(sess *lifecycleSession) (accepted bool) {
	s.mu.Lock()
	if s.taken {
		s.mu.Unlock()
		// The main path has already looked and moved on, so no one else will
		// ever close this. Closing it here, off the lock, is the only thing
		// standing between a raced quit and live credentials on the heap.
		sess.close()
		return false
	}
	prev := s.session
	s.session = sess
	s.mu.Unlock()

	// The flow prev backed was escaped out of; only one can be live at a time.
	if prev != nil {
		prev.close()
	}
	return true
}

// take closes the slot to further registrations and returns whatever session it
// holds; every put after it closes its own session.
func (s *lifecycleSlot) take() *lifecycleSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.taken = true
	return s.session
}
