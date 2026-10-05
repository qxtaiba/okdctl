package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/node"
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
	err := reportLifecycleOutcome(cmd, wizard.Result{Outcome: wizard.OutcomeCancelled}, st)
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
	if err := reportLifecycleOutcome(cmd, wizard.Result{Outcome: wizard.OutcomeCompleted}, st); err != nil {
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

// TestLifecycleRunFlowErrMapsRendererFailureToInterrupted guards the
// renderer-failure execution state: a wizard.RunFlow error (a tea.Program
// crash, e.g.) arriving mid-execution — Started but not yet Executed — must
// map to the same interrupted/resume guidance as a graceful cancel, never
// read as a configuration problem with nothing to resume.
func TestLifecycleRunFlowErrMapsRendererFailureToInterrupted(t *testing.T) {
	boom := errors.New("tea: renderer panicked")
	st := &lifecycle.State{Started: true, Executed: false}

	err := lifecycleRunFlowErr(boom, st)

	var ce *errtypes.ClusterError
	if !errors.As(err, &ce) {
		t.Fatalf("renderer failure mid-execution must be a *errtypes.ClusterError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "resume") {
		t.Errorf("renderer failure mid-execution must point at the resume marker: %v", err)
	}
	if !errors.Is(err, boom) {
		t.Errorf("the underlying renderer error must still be wrapped: %v", err)
	}
}

// TestLifecycleRunFlowErrMapsPreExecutionFailureToConfigError guards the
// other half: a RunFlow failure before execution ever started carries no
// marker to resume, so it must read as a configuration problem, not an
// interrupted operation.
func TestLifecycleRunFlowErrMapsPreExecutionFailureToConfigError(t *testing.T) {
	boom := errors.New("tea: could not open a new tty")
	st := &lifecycle.State{Started: false, Executed: false}

	err := lifecycleRunFlowErr(boom, st)

	var ce *errtypes.ConfigError
	if !errors.As(err, &ce) {
		t.Fatalf("pre-execution renderer failure must be a *errtypes.ConfigError, got %T: %v", err, err)
	}
	if strings.Contains(err.Error(), "resume") {
		t.Errorf("pre-execution failure must not claim a resumable marker: %v", err)
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
	if err := reportLifecycleOutcome(cmd, wizard.Result{Outcome: wizard.OutcomeCompleted}, st); !errors.Is(err, boom) {
		t.Errorf("failed run must propagate the backend error, got %v", err)
	}
}

func TestReportLifecycleOutcomeNoConsentMeansNoChanges(t *testing.T) {
	cmd, out := outcomeCmd()
	st := &lifecycle.State{Proceed: false}
	if err := reportLifecycleOutcome(cmd, wizard.Result{Outcome: wizard.OutcomeCancelled}, st); err != nil {
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
