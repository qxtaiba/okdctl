package render

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/postinstall"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/tui"
)

// Every box returns "\n"+box+"\n" and gets a single blank line after it from
// whatever follows: a caller printing a box with nothing but a plain return,
// prompt, or end of output after it must use Fprintln, but a caller printing
// two boxes back to back (see internal/cli/deploy.go's printDeployDryRunBoxes)
// must print the earlier one with plain Fprint — the later box's own leading
// "\n" is what supplies that single blank line, and Fprintln-ing both would
// double it.
func TestBoxFramingContract(t *testing.T) {
	tui.SetColorProfileFor(&bytes.Buffer{})
	t.Cleanup(func() { tui.SetColorProfileFor(&bytes.Buffer{}) })

	steps := []distribution.StepResult{{StepID: "download-tools", Success: true, Duration: time.Second}}
	drift := []terraform.ResourceChange{{Address: "module.vm.worker[2]", Action: terraform.PlanActionUpdate}}
	removeP := removePlan()
	addP := addPlan()

	cases := []struct {
		name string
		box  func() string
	}{
		{"ErrorSummary", func() string {
			return ErrorSummary(&errtypes.ConfigError{Msg: "boom"}, 1, "run-1")
		}},
		{"ErrorCard", func() string {
			return ErrorCard("config error", "boom", "try again", tui.DefaultBoxWidth)
		}},
		{"NodeOpConfirm", func() string { return NodeOpConfirm(&removeP) }},
		{"NodeOpDryRun", func() string { return NodeOpDryRun(&removeP) }},
		{"NodeOpComplete", func() string { return NodeOpComplete(&addP, time.Minute) }},
		{"NodeOpCompleteWidth", func() string { return NodeOpCompleteWidth(&addP, time.Minute, 70) }},
		{"PlanPreview clean", func() string { return PlanPreview(nil) }},
		{"PlanPreview drifted", func() string { return PlanPreview(drift) }},
		{"DryRunSummary", func() string {
			return DryRunSummary("deploy step listing", []DryRunStep{{ID: "download-tools", Name: "download tools"}})
		}},
		{"PostDeploySummary", func() string {
			return PostDeploySummary(config.DefaultConfig(), &postinstall.Result{}, steps, "run-1")
		}},
		{"InterruptSummary", func() string {
			return InterruptSummary(steps, "okdctl deploy", "run-1")
		}},
		{"FailureSummary", func() string {
			return FailureSummary(&FailureInfo{
				Steps: steps, Phase: "install", RunID: "run-1",
				TeardownCmd: "okdctl destroy", TeardownNote: "remove provisioned resources",
			})
		}},
		{"UpdateIngressSummary", func() string {
			return UpdateIngressSummary(&postinstall.UpdateIngressResult{})
		}},
		{"ConfirmBox irreversible", func() string {
			return ConfirmBox("destroy", []Fact{{Key: "cluster", Value: "grappleberry"}}, IrreversibleWarning)
		}},
		{"ConfirmBox reversible", func() string {
			return ConfirmBox("ingress update", []Fact{{Key: "cluster", Value: "grappleberry"}}, "")
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.box()
			if !strings.HasPrefix(got, "\n") || strings.HasPrefix(got, "\n\n") {
				t.Errorf("%s must start with exactly one leading newline:\n%q", tc.name, got)
			}
			if !strings.HasSuffix(got, "╯\n") {
				t.Errorf("%s must end with the box's closing corner followed by one newline:\n%q", tc.name, got)
			}
			if strings.HasSuffix(got, "\n\n") {
				t.Errorf("%s must not carry a trailing blank line inside the returned box string:\n%q", tc.name, got)
			}
		})
	}
}
