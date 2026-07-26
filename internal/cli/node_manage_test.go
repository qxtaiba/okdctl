package cli

import (
	"bytes"
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
