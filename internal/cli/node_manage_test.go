package cli

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/lifecycle"
)

func outcomeCmd() (*cobra.Command, *bytes.Buffer) {
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	return cmd, &out
}

func TestRunLifecycleOpRejectsUnknownOp(t *testing.T) {
	rc := &nodeRunnerCtx{runner: &node.Runner{}}
	st := &lifecycle.State{Op: node.Op("bogus")}
	err := runLifecycleOp(context.Background(), rc, st)
	var usageErr *errtypes.UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("unknown lifecycle op must be *errtypes.UsageError, got %T: %v", err, err)
	}
}

func TestReportLifecycleOutcomeInterruptedIsNotSilent(t *testing.T) {
	cmd, out := outcomeCmd()
	st := &lifecycle.State{Cfg: config.DefaultConfig(), Proceed: true, Started: true}
	err := reportLifecycleOutcome(cmd, wizard.Result{Cancelled: true}, st)
	if err == nil {
		t.Fatal("an interrupted execution must exit non-zero, never 'no changes made'")
	}
	var ce *errtypes.ClusterError
	if !errors.As(err, &ce) || !strings.Contains(err.Error(), "resume") {
		t.Errorf("interrupted outcome must point at the resume marker: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("no completion box on an interrupted run, got %q", out.String())
	}
}

// TestReportLifecycleOutcomeSuccessPrintsRecap supersedes the former
// TestReportLifecycleOutcomeExecutedPaths, which asserted success prints
// nothing ("the done screen already showed the box"). That ruling is
// reversed: the AltScreen clears the done card from scrollback on exit, so
// success now prints a short plain recap (item 5, second-cut safety
// findings) — this test honestly updates the old expectation rather than
// silently deleting it.
func TestReportLifecycleOutcomeSuccessPrintsRecap(t *testing.T) {
	plan := &node.OpPlan{
		Op: node.OpResize, Cluster: "homelab",
		Nodes: []node.PlanNode{{Name: "m0", Role: "master", Action: terraform.PlanActionUpdate}},
	}

	cmd, out := outcomeCmd()
	st := &lifecycle.State{Proceed: true, Started: true, Executed: true, Plan: plan, Elapsed: 90 * time.Second}
	if err := reportLifecycleOutcome(cmd, wizard.Result{Completed: true}, st); err != nil {
		t.Fatalf("successful run: %v", err)
	}
	got := out.String()
	for _, want := range []string{"resize complete", "m0", "1m30s"} {
		if !strings.Contains(got, want) {
			t.Errorf("success recap missing %q, got %q", want, got)
		}
	}
	for _, step := range render.NodeOpNextSteps(plan) {
		if !strings.Contains(got, step) {
			t.Errorf("success recap missing next-step line %q, got %q", step, got)
		}
	}
}

func TestReportLifecycleOutcomeFailurePropagatesBackendError(t *testing.T) {
	plan := &node.OpPlan{
		Op: node.OpResize, Cluster: "homelab",
		Nodes: []node.PlanNode{{Name: "m0", Role: "master", Action: terraform.PlanActionUpdate}},
	}
	boom := errors.New("etcd gate failed")
	cmd, _ := outcomeCmd()
	st := &lifecycle.State{Proceed: true, Started: true, Executed: true, Plan: plan, Result: boom}
	if err := reportLifecycleOutcome(cmd, wizard.Result{Completed: true}, st); !errors.Is(err, boom) {
		t.Errorf("failed run must propagate the backend error, got %v", err)
	}
}

func TestReportLifecycleOutcomeNoConsentMeansNoChanges(t *testing.T) {
	cmd, out := outcomeCmd()
	st := &lifecycle.State{Proceed: false}
	if err := reportLifecycleOutcome(cmd, wizard.Result{Cancelled: true}, st); err != nil {
		t.Fatalf("backing out pre-consent must exit clean: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("no completion box pre-consent, got %q", out.String())
	}
}

// TestResizeOptsFromWizardMergesHostAndDatastoreBudget guards the wizard
// dispatch path (runLifecycleOp) merging the read-only Proxmox probe results
// onto the wizard-collected resize options the same way the flag verb
// (runNodeResize) does — a dropped merge here arms the memory guard but
// leaves the datastore guard disarmed for every TUI-driven disk resize.
func TestResizeOptsFromWizardMergesHostAndDatastoreBudget(t *testing.T) {
	rc := &nodeRunnerCtx{HostTotalMiB: 65536, HostAllocatedMiB: 32768, DatastoreAvailGB: 500}
	st := &lifecycle.State{MemoryMB: 16384, OSDiskGB: 100}

	opts := resizeOptsFromWizard(rc, st)

	if opts.HostTotalMiB != 65536 || opts.HostAllocatedMiB != 32768 {
		t.Errorf("host memory budget not merged onto wizard resize options: %+v", opts)
	}
	if opts.DatastoreAvailGB != 500 {
		t.Errorf("datastore budget not merged onto wizard resize options: %+v", opts)
	}
	if opts.MemoryMB != 16384 || opts.OSDiskGB != 100 {
		t.Errorf("wizard-collected dimensions lost in the merge: %+v", opts)
	}
}

// TestAddOptsFromWizardMergesHostBudget mirrors the resize case for node
// add, whose wizard path merges the same memory-budget probe.
func TestAddOptsFromWizardMergesHostBudget(t *testing.T) {
	rc := &nodeRunnerCtx{HostTotalMiB: 65536, HostAllocatedMiB: 32768}
	st := &lifecycle.State{Count: 2}

	opts := addOptsFromWizard(rc, st)

	if opts.HostTotalMiB != 65536 || opts.HostAllocatedMiB != 32768 {
		t.Errorf("host memory budget not merged onto wizard add options: %+v", opts)
	}
	if opts.Count != 2 {
		t.Errorf("wizard-collected count lost in the merge: %+v", opts)
	}
}

// TestSendExecEventDeliversAfterGracefulCancel pins biased delivery on the
// runner's Reporter/OnStep seam: gate transitions racing the cancel must
// keep landing while the exec screen is still draining the feed.
func TestSendExecEventDeliversAfterGracefulCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	for range 200 {
		events := make(chan lifecycle.ExecEvent, 1)
		sendExecEvent(ctx, events, &lifecycle.ExecEvent{Desc: "drain node"})
		select {
		case <-events:
		default:
			t.Fatal("a graceful cancel dropped an exec event despite buffer space")
		}
	}
}

func TestAccessibleNodeManageUsesDryRunAndDefaultsToNoExecute(t *testing.T) {
	state := &lifecycle.State{Cfg: config.DefaultConfig()}
	var dryRun, execute int
	sess := &lifecycleSession{
		state: state,
		hooks: lifecycle.Hooks{
			ListNodes: func() ([]cluster.NodeDetail, error) {
				return []cluster.NodeDetail{
					{Name: "worker0", Role: nodetypes.RoleWorker, Ready: true},
				}, nil
			},
			DryRun: func(st *lifecycle.State) (*node.OpPlan, error) {
				dryRun++
				return &node.OpPlan{Op: st.Op, Cluster: st.Cfg.Cluster.Name}, nil
			},
			Execute: func(*lifecycle.State, chan<- lifecycle.ExecEvent) error {
				execute++
				return nil
			},
		},
	}
	prompt := &scriptedAccessiblePrompt{values: map[string]string{
		"operation (resume/resize/add/remove; blank cancels)": "add",
		"workers to add": "2",
	}}
	if err := runAccessibleNodeManageWith(context.Background(), prompt, sess); err != nil {
		t.Fatal(err)
	}
	if dryRun != 1 || execute != 0 {
		t.Fatalf("dry-run calls = %d, execute calls = %d; want 1 and 0", dryRun, execute)
	}
	if state.Op != node.OpAdd || state.Count != 2 || state.Proceed {
		t.Fatalf("state after review = %+v", state)
	}
	if !strings.Contains(prompt.output.String(), "Plan review") {
		t.Fatalf("plan review missing: %q", prompt.output.String())
	}
	if !slices.Contains(prompt.linePrompts, "execute this plan? (y/N)") {
		t.Fatalf("execute confirmation was not prompted: %v", prompt.linePrompts)
	}
}

func TestAccessibleNodeManageRequiresTypedClusterNameForDataDestruction(t *testing.T) {
	state := &lifecycle.State{Cfg: config.DefaultConfig()}
	var execute int
	sess := &lifecycleSession{
		state: state,
		hooks: lifecycle.Hooks{
			ListNodes: func() ([]cluster.NodeDetail, error) {
				return []cluster.NodeDetail{{Name: "worker0", Role: nodetypes.RoleWorker, Ready: true}}, nil
			},
			DryRun: func(st *lifecycle.State) (*node.OpPlan, error) {
				return &node.OpPlan{
					Op: st.Op, Cluster: st.Cfg.Cluster.Name,
					Nodes: []node.PlanNode{{Name: "worker0", Action: terraform.PlanActionDelete}},
				}, nil
			},
			Execute: func(*lifecycle.State, chan<- lifecycle.ExecEvent) error {
				execute++
				return nil
			},
		},
	}
	prompt := &scriptedAccessiblePrompt{values: map[string]string{
		"operation (resume/resize/add/remove; blank cancels)": "remove",
		"worker to remove (highest-numbered worker only)":     "worker0",
		"drain mode (drain/skip)":                             "drain",
		"drain timeout":                                       "10m",
		"force removal with storage data loss? (y/N)":         "no",
		"type cluster name to confirm destruction":            "wrong-name",
	}}
	if err := runAccessibleNodeManageWith(context.Background(), prompt, sess); err == nil {
		t.Fatal("mismatched cluster name accepted")
	}
	if execute != 0 {
		t.Fatalf("execute called %d times after a mismatched name", execute)
	}
}

func TestAccessibleNodeManageExecutesOnlyAfterReviewedPlanConfirmation(t *testing.T) {
	state := &lifecycle.State{Cfg: config.DefaultConfig()}
	executed := false
	sess := &lifecycleSession{
		state: state,
		hooks: lifecycle.Hooks{
			ListNodes: func() ([]cluster.NodeDetail, error) {
				return []cluster.NodeDetail{{Name: "worker0", Role: nodetypes.RoleWorker, Ready: true}}, nil
			},
			DryRun: func(st *lifecycle.State) (*node.OpPlan, error) {
				return &node.OpPlan{
					Op: st.Op, Cluster: st.Cfg.Cluster.Name,
					Nodes: []node.PlanNode{{Name: "worker0", Role: nodetypes.RoleWorker, Action: terraform.PlanActionUpdate}},
				}, nil
			},
			Execute: func(_ *lifecycle.State, events chan<- lifecycle.ExecEvent) error {
				executed = true
				events <- lifecycle.ExecEvent{Node: "worker0", Step: node.StepPowerCycle}
				return nil
			},
		},
	}
	prompt := &scriptedAccessiblePrompt{values: map[string]string{
		"operation (resume/resize/add/remove; blank cancels)": "resize",
		"target (masters, workers, or node name)":             "workers",
		"memory (mb; 0 keeps current)":                        "16384",
		"vcpus (0 keeps current)":                             "0",
		"os disk (gb; 0 keeps current, grow-only)":            "0",
		"drain mode (drain/skip)":                             "drain",
		"drain timeout":                                       "10m",
		"execute this plan? (y/N)":                            "y",
	}}
	if err := runAccessibleNodeManageWith(context.Background(), prompt, sess); err != nil {
		t.Fatal(err)
	}
	if !executed || !state.Started || !state.Executed || state.Result != nil {
		t.Fatalf("execution state: executed=%t state=%+v", executed, state)
	}
	for _, want := range []string{"worker0: power-cycle", "resize complete"} {
		if !strings.Contains(prompt.output.String(), want) {
			t.Errorf("output missing %q: %q", want, prompt.output.String())
		}
	}
}

func TestNodeManageHelpNamesAccessibleMode(t *testing.T) {
	if got := nodeManageCmd.Flags().Lookup("accessible"); got == nil {
		t.Fatal("node manage is missing --accessible")
	}
	if !strings.Contains(nodeManageCmd.Long, "OKDCTL_ACCESSIBLE=1") {
		t.Fatal("node manage help is missing the environment-variable equivalent")
	}
}
