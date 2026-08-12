package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/lifecycle"
)

func outcomeCmd() (*cobra.Command, *bytes.Buffer) {
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	return cmd, &out
}

// TestRunLifecycleOpRejectsUnknownOp locks the dispatch table's default arm:
// an unrecognized op must fail as a *errtypes.UsageError (exit 64) rather than
// silently no-op or run the wrong destructive branch against the target. The
// three real ops route to RemoveWorker/Resize/AddWorkers, exercised end-to-end
// by the node package's remove/resize/add tests.
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

func TestReportLifecycleOutcomeExecutedPaths(t *testing.T) {
	plan := &node.OpPlan{
		Op: node.OpResize, Cluster: "homelab",
		Nodes: []node.PlanNode{{Name: "m0", Role: "master", Action: terraform.PlanActionUpdate}},
	}

	cmd, out := outcomeCmd()
	st := &lifecycle.State{Proceed: true, Started: true, Executed: true, Plan: plan}
	if err := reportLifecycleOutcome(cmd, wizard.Result{Completed: true}, st); err != nil {
		t.Fatalf("successful run: %v", err)
	}
	if !strings.Contains(out.String(), "resize") {
		t.Error("successful run must print the completion box")
	}

	boom := errors.New("etcd gate failed")
	cmd, _ = outcomeCmd()
	st = &lifecycle.State{Proceed: true, Started: true, Executed: true, Plan: plan, Result: boom}
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
