package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/clusterstatus"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/lifecycle"
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

	env, err := prepareNodeOpsEnv(ctx, cfg, true)
	if err != nil {
		return err
	}
	defer env.close()

	cl, err := clusterstatus.NewClient(env.projectRoot)
	if err != nil {
		return err
	}

	marker, err := node.ReadOpMarker(filepath.Join(env.projectRoot, "okd-install"), cfg.Cluster.Name)
	if err != nil {
		return err
	}

	st := &lifecycle.State{Cfg: cfg, Marker: marker}
	hooks := lifecycle.Hooks{
		ListNodes: func() ([]cluster.NodeDetail, error) { return cl.ListNodes(ctx) },
		DryRun: func(s *lifecycle.State) (*node.OpPlan, error) {
			rc, err := env.newRunner(cmd, cfg, "manage", nodeConsent{dryRun: true})
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
	}

	result, err := wizard.RunFlow(ctx, lifecycle.NewSteps(st, hooks), cfg, lifecycleChrome())
	if err != nil {
		return &errtypes.ConfigError{Msg: "lifecycle wizard", Err: err}
	}
	if result.Cancelled || !st.Proceed {
		tui.Info("no changes made")
		return nil
	}

	return executeLifecycleOp(cmd, cfg, env, st)
}

// executeLifecycleOp runs the wizard-approved operation on the plain
// terminal (spinners + informed box), with consent collapsed into the
// wizard screens: the runner's ConfirmFunc only cross-checks that the
// world still matches the plan the operator approved.
func executeLifecycleOp(cmd *cobra.Command, cfg *config.Config, env *nodeOpsEnv, st *lifecycle.State) error {
	rc, err := env.newRunner(cmd, cfg, "manage", nodeConsent{})
	if err != nil {
		return err
	}
	defer rc.cleanup()

	approved := st.Plan
	errW := cmd.ErrOrStderr()
	rc.runner.Confirm = func(_ context.Context, p *node.OpPlan) (bool, error) {
		rc.captured = p
		fmt.Fprint(errW, render.NodeOpConfirm(p))
		if !lifecycle.PlansEquivalent(approved, p) {
			return false, &errtypes.ClusterError{Msg: "the cluster changed since the preview — re-run 'okdctl node manage' to re-plan"}
		}
		return true, nil
	}

	start := time.Now()
	if err := runLifecycleOp(cmd.Context(), rc, st); err != nil {
		if errors.Is(err, node.ErrDeclined) {
			return nil
		}
		return err
	}
	rc.complete(cmd.OutOrStdout(), time.Since(start))
	return nil
}

// runLifecycleOp dispatches the wizard-collected operation onto the
// runner, merging the host-probe budget the same way the flag verbs do.
func runLifecycleOp(ctx context.Context, rc *nodeRunnerCtx, st *lifecycle.State) error {
	switch st.Op {
	case node.OpResize:
		opts := lifecycle.ResizeOptionsFrom(st)
		opts.HostTotalMiB, opts.HostAllocatedMiB = rc.HostTotalMiB, rc.HostAllocatedMiB
		return rc.runner.Resize(ctx, st.Scope, opts)
	case node.OpAdd:
		opts := lifecycle.AddOptionsFrom(st)
		opts.HostTotalMiB, opts.HostAllocatedMiB = rc.HostTotalMiB, rc.HostAllocatedMiB
		return rc.runner.AddWorkers(ctx, opts)
	case node.OpRemove:
		return rc.runner.RemoveWorker(ctx, st.Target, lifecycle.RemoveOptionsFrom(st))
	default:
		return &errtypes.UsageError{Msg: fmt.Sprintf("unsupported lifecycle op %q", st.Op)}
	}
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
