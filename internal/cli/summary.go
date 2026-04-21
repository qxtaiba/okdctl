package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/postinstall"
	"github.com/qxtaiba/okdctl/internal/tui"
)

const (
	defaultContentWidth = tui.DefaultBoxWidth - 2
	defaultKeyColWidth  = 45
)

// stepDisplayStatus is the three-state tag printed next to each step
// line in the summary.
type stepDisplayStatus string

const (
	stepStatusSkip stepDisplayStatus = "skip"
	stepStatusOK   stepDisplayStatus = "ok"
	stepStatusFail stepDisplayStatus = "fail"
)

func displayStatus(s *distribution.StepResult) stepDisplayStatus {
	switch {
	case s.Skipped:
		return stepStatusSkip
	case s.Success:
		return stepStatusOK
	default:
		return stepStatusFail
	}
}

type summaryBuilder struct {
	b        strings.Builder
	keyWidth int
	kvWidth  int
}

func newSummaryBuilder() *summaryBuilder {
	return &summaryBuilder{
		keyWidth: defaultKeyColWidth,
		kvWidth:  defaultContentWidth - 2,
	}
}

func (s *summaryBuilder) section(title string) {
	s.b.WriteString("  " + tui.SubsectionLabel(title) + "\n")
}

func (s *summaryBuilder) kv(key, value string) {
	s.b.WriteString("  " + tui.DottedKeyValueFull("  "+key, value, s.keyWidth, s.kvWidth) + "\n")
}

func (s *summaryBuilder) kvHighlight(key, value string) {
	s.b.WriteString("  " + tui.DottedKeyValueHighlightFull("  "+key, value, s.keyWidth, s.kvWidth) + "\n")
}

func (s *summaryBuilder) newline() {
	s.b.WriteString("\n")
}

func (s *summaryBuilder) String() string {
	return s.b.String()
}

// DryRunStep is a single step entry for a dry-run plan listing.
type DryRunStep struct {
	ID   string
	Name string
}

// DryRunSummary renders the step listing for a dry-run inside a boxed section
// consistent with PostDeploySummary.
func DryRunSummary(title string, steps []DryRunStep) string {
	sb := newSummaryBuilder()
	sb.b.WriteString("\n")
	sb.b.WriteString("  " + tui.WarningStyle.Render("dry-run — no changes made") + "\n")
	sb.newline()

	if len(steps) > 0 {
		sb.section("would execute")
		for _, s := range steps {
			sb.kv(s.ID, s.Name)
		}
		sb.newline()
	}

	return "\n" + tui.BoxedSectionCompact(sb.String(), title, tui.DefaultBoxWidth) + "\n"
}

// ValidationSummary renders a config validation result for CLI output,
// listing each error with field context when present.
func ValidationSummary(result *config.ValidationResult) string {
	var sb strings.Builder

	if result.IsValid() {
		sb.WriteString(tui.CompletionSuccess("configuration is valid"))
	} else {
		sb.WriteString(tui.CompletionError(fmt.Sprintf("configuration invalid (%d errors)", len(result.Errors))))
		sb.WriteString("\n\n")
		for _, e := range result.Errors {
			sb.WriteString("  " + tui.ErrorStyle.Render(tui.IconError+" [fail]") + " ")
			if e.Field != "" {
				sb.WriteString(tui.MutedStyle.Render(e.Field + ": "))
			}
			sb.WriteString(e.Message + "\n")
		}
	}

	return sb.String()
}

// PostDeploySummary renders the success summary shown after a cluster deploy
// completes, including access URLs, kubeadmin credentials, and step results.
func PostDeploySummary(cfg *config.Config, result *postinstall.Result, steps []distribution.StepResult, runID string) string {
	clusterFQDN := cfg.Cluster.Name + "." + cfg.Cluster.Domain
	consoleURL := fmt.Sprintf("https://console-openshift-console.apps.%s", clusterFQDN)
	apiURL := fmt.Sprintf("https://api.%s:6443", clusterFQDN)

	sb := newSummaryBuilder()
	sb.b.WriteString("\n")
	sb.b.WriteString("  " + tui.CompletionSuccess("cluster deployed successfully!") + "\n")
	sb.newline()
	sb.kv("run_id", runID)
	sb.newline()

	sb.section("access")
	sb.kv("cluster", clusterFQDN)
	sb.kv("console", consoleURL)
	sb.kv("api", apiURL)
	sb.newline()

	sb.section("dns records")
	apiDomain := fmt.Sprintf("api.%s", clusterFQDN)
	appsDomain := fmt.Sprintf("*.apps.%s", clusterFQDN)
	if result != nil && result.DNSDeployed && result.KubeVipIP != "" {
		sb.kv(apiDomain, result.KubeVipIP+" (kube-vip)")
	} else if cfg.Networking.Bastion.IP != "" {
		sb.kv(apiDomain, cfg.Networking.Bastion.IP+" (haproxy)")
	}
	bastionIP := cfg.Networking.Bastion.IP
	if result != nil && result.BastionIP != "" {
		bastionIP = result.BastionIP
	}
	sb.kv(appsDomain, bastionIP+" (haproxy)")
	sb.newline()

	sb.section("status")
	if result != nil {
		if result.BootstrapCleaned {
			sb.kv("bootstrap", "cleaned up")
		} else {
			sb.kv("bootstrap", "still running")
		}
		if result.DNSDeployed && result.KubeVipIP != "" {
			sb.kv("api routing", fmt.Sprintf("kube-vip (%s)", result.KubeVipIP))
		} else {
			sb.kv("api routing", "haproxy (bastion)")
		}
		sb.kv("ingress routing", "haproxy (bastion)")
	}
	sb.newline()

	if len(steps) > 0 {
		sb.section("steps")
		var total time.Duration
		for _, s := range steps {
			total += s.Duration
			d := s.Duration.Truncate(time.Millisecond).String()
			sb.kv(string(s.StepID), fmt.Sprintf("%-4s  %s", displayStatus(&s), d))
		}
		sb.kv("total", total.Truncate(time.Millisecond).String())
		sb.newline()
	}

	sb.section("credentials")
	sb.kvHighlight("username", "kubeadmin")
	sb.kv("password", "cat okd-install/cluster-config/auth/kubeadmin-password")
	sb.newline()

	sb.section("quick start")
	sb.b.WriteString("    " + tui.CodeInlineStyle.Render("export KUBECONFIG=~/.kube/config") + "\n")
	sb.b.WriteString("    " + tui.CodeInlineStyle.Render("oc get nodes") + "\n")
	sb.newline()

	sb.section("next steps")
	sb.b.WriteString("    cluster deployed with haproxy handling ingress on the bastion.\n")
	sb.b.WriteString("    if you deploy a loadbalancer provider (e.g., metallb), run:\n")
	sb.b.WriteString("      " + tui.CodeInlineStyle.Render("okdctl update-ingress") + "\n")
	sb.b.WriteString("    to auto-detect loadbalancer ips and switch dns over.\n")
	sb.newline()

	return "\n" + tui.BoxedSectionCompact(sb.String(), "deployment complete", tui.DefaultBoxWidth) + "\n"
}

// InterruptSummary renders a partial-progress box for a Ctrl-C interruption.
// steps is whatever the orchestrator completed before cancellation;
// resumeCmd is the exact command the user should re-run (e.g. "okdctl deploy").
func InterruptSummary(steps []distribution.StepResult, resumeCmd, runID string) string {
	sb := newSummaryBuilder()
	sb.b.WriteString("\n")
	sb.b.WriteString("  " + tui.WarningStyle.Render("interrupted") + "\n")
	sb.newline()
	sb.kv("run_id", runID)
	sb.newline()

	if len(steps) > 0 {
		sb.section("partial progress")
		for _, s := range steps {
			d := s.Duration.Truncate(time.Millisecond).String()
			sb.kv(string(s.StepID), fmt.Sprintf("%-4s  %s", displayStatus(&s), d))
		}
		sb.newline()
	}

	sb.section("resume")
	sb.b.WriteString("    " + tui.CodeInlineStyle.Render(resumeCmd) + "\n")
	sb.newline()

	return "\n" + tui.BoxedSectionCompact(sb.String(), "interrupted", tui.DefaultBoxWidth) + "\n"
}

// UpdateIngressSummary renders the result of the update-ingress subcommand,
// showing converted controllers and DNS record changes.
func UpdateIngressSummary(result *postinstall.UpdateIngressResult) string {
	sb := newSummaryBuilder()
	sb.newline()

	if result.ConvertedCount > 0 {
		sb.section("conversion")
		sb.kv("controllers converted", fmt.Sprintf("%d (HostNetwork → LoadBalancerService)", result.ConvertedCount))
		sb.newline()
	}

	sb.section("dns records")
	if result.KubeVipIP != "" {
		sb.kvHighlight("api.*", result.KubeVipIP+" (kube-vip)")
	}
	for _, e := range result.Entries {
		label := fmt.Sprintf("*.%s", e.Domain)
		var suffix string
		switch {
		case e.HostNetwork:
			suffix = " (bastion)"
		case e.Converted:
			suffix = " (loadbalancer, converted)"
		default:
			suffix = " (loadbalancer)"
		}
		sb.kvHighlight(label, e.LBIP+suffix)
	}
	sb.newline()

	sb.section("status")
	if result.HAProxyRemoved {
		sb.kv("haproxy", "stopped and disabled")
	} else {
		sb.kv("haproxy", "still running")
	}
	sb.newline()

	return "\n" + tui.BoxedSectionCompact(sb.String(), "ingress updated", tui.DefaultBoxWidth) + "\n"
}
