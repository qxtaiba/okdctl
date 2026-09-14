// Package render builds the boxed text summaries okdctl prints after deploys,
// dry-runs, and interruptions.
package render

import (
	"fmt"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/postinstall"
	"github.com/qxtaiba/okdctl/internal/tui"
)

const defaultKeyColWidth = 45

// minWrapWidth floors the wrap width for Para and Bullet so a narrow box
// never collapses prose to an unreadable sliver.
const minWrapWidth = 12

type stepDisplayStatus string

const (
	stepStatusSkip stepDisplayStatus = "skip"
	stepStatusOK   stepDisplayStatus = "ok"
	stepStatusFail stepDisplayStatus = "fail"
)

// stepStatusColWidth must equal max(len(stepStatus*)) so values align without truncation.
const stepStatusColWidth = 4

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

// Builder accumulates aligned section/key-value lines for a boxed summary.
type Builder struct {
	b        strings.Builder
	keyWidth int
	kvWidth  int
}

// NewBuilder returns a Builder with the shared summary column widths.
func NewBuilder() *Builder {
	return NewBuilderWidth(tui.DefaultBoxWidth)
}

// NewBuilderWidth returns a Builder whose key and key/value columns fit
// inside a box of the given width.
func NewBuilderWidth(width int) *Builder {
	content := tui.BoxInnerWidth(width) - 2
	return &Builder{
		keyWidth: min(defaultKeyColWidth, content-24),
		kvWidth:  content,
	}
}

// ContentWidth reports the content width available for raw lines inside this Builder's box.
func (s *Builder) ContentWidth() int {
	return s.kvWidth
}

// Section writes a subsection label line.
func (s *Builder) Section(title string) {
	s.b.WriteString("  " + tui.SubsectionLabel(title) + "\n")
}

// kvOpts configures the private kv core shared by every KV* row.
type kvOpts struct {
	keyColWidth int // 0 = use the Builder's default keyWidth
	highlight   bool
	sub         bool
}

// kv renders key/value through the tui dotted-line variant opts selects and
// writes the result, indenting every line (including any wrapped
// continuation) by the Builder's two-space margin.
func (s *Builder) kv(key, value string, opts kvOpts) {
	keyColWidth := opts.keyColWidth
	if keyColWidth <= 0 {
		keyColWidth = s.keyWidth
	}

	var rendered string
	switch {
	case opts.sub:
		rendered = tui.DottedKeyValueSubFull("  "+key, value, keyColWidth, s.kvWidth)
	case opts.highlight:
		rendered = tui.DottedKeyValueHighlightFull("  "+key, value, keyColWidth, s.kvWidth)
	default:
		rendered = tui.DottedKeyValueFull("  "+key, value, keyColWidth, s.kvWidth)
	}
	s.writeIndented(rendered)
}

// writeIndented writes rendered — one or more newline-joined lines — each
// prefixed by the Builder's two-space margin.
func (s *Builder) writeIndented(rendered string) {
	for i, line := range strings.Split(rendered, "\n") {
		if i > 0 {
			s.b.WriteString("\n")
		}
		s.b.WriteString("  " + line)
	}
	s.b.WriteString("\n")
}

// writeWrapped writes text word-wrapped to fit the box, led by prefix on the
// first line and by indent spaces on every continuation; extra is how many
// columns prefix costs beyond the Builder's baseline two-space margin.
func (s *Builder) writeWrapped(prefix string, indent, extra int, text string) {
	width := max(s.kvWidth-extra, minWrapWidth)
	for i, line := range tui.WrapLines(text, width) {
		if i == 0 {
			s.b.WriteString(prefix + line + "\n")
		} else {
			s.b.WriteString(strings.Repeat(" ", indent) + line + "\n")
		}
	}
}

// KV writes a dotted key/value line.
func (s *Builder) KV(key, value string) {
	s.kv(key, value, kvOpts{})
}

// KVHighlight writes a dotted key/value line with a highlighted value.
func (s *Builder) KVHighlight(key, value string) {
	s.kv(key, value, kvOpts{highlight: true})
}

// KVWide writes a dotted key/value line with a 24-column key, leaving long command or URL values more room before they wrap.
func (s *Builder) KVWide(key, value string) {
	s.kv(key, value, kvOpts{keyColWidth: tui.DefaultKeyColWidth})
}

// SubKV writes a nested dotted key/value line with a muted key and a key column narrowed by two.
func (s *Builder) SubKV(key, value string) {
	s.kv(key, value, kvOpts{keyColWidth: s.keyWidth - 2, sub: true})
}

// Note writes a labeled prose line with no dot leaders, wrapping the text under the value column.
func (s *Builder) Note(label, text string) {
	s.writeIndented(tui.KeyValueNote("  "+label, text, s.keyWidth, s.kvWidth))
}

// Para writes a word-wrapped paragraph indented two spaces.
func (s *Builder) Para(text string) {
	s.writeWrapped("  ", 2, 0, text)
}

// Bullet writes a bullet-prefixed, word-wrapped line with continuations aligned under the text.
func (s *Builder) Bullet(text string) {
	s.writeWrapped("    "+tui.IconBullet+" ", 6, 4, text)
}

// Table writes tui.Table's rendered lines indented four spaces.
func (s *Builder) Table(headers []string, rows [][]string, opts tui.TableOptions) {
	for _, line := range tui.Table(headers, rows, opts) {
		s.b.WriteString("    " + tui.Downsample(line) + "\n")
	}
}

// Newline writes a blank spacer line between sections.
func (s *Builder) Newline() {
	s.b.WriteString("\n")
}

// WriteString appends raw text without key/section formatting.
func (s *Builder) WriteString(str string) {
	s.b.WriteString(str)
}

func (s *Builder) String() string {
	return s.b.String()
}

// DryRunStep is a single step entry for a dry-run plan listing.
type DryRunStep struct {
	ID   string
	Name string
}

// DryRunSummary renders the step listing for a dry-run, styled like PostDeploySummary.
func DryRunSummary(title string, steps []DryRunStep) string {
	sb := NewBuilder()
	sb.WriteString("\n")
	sb.WriteString("  " + tui.WarningStyle.Render("dry-run — no changes made") + "\n")
	sb.Newline()

	if len(steps) > 0 {
		sb.Section("would execute")
		for _, s := range steps {
			sb.KV(s.ID, s.Name)
		}
		sb.Newline()
	}

	return "\n" + tui.BoxedSectionCompact(sb.String(), title, tui.DefaultBoxWidth) + "\n"
}

// ValidationSummary renders a config validation result, listing each error with
// field context when present.
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

	return tui.Downsample(sb.String())
}

// PostDeploySummary renders the success summary after a cluster deploy: access
// URLs, credentials, and step results.
func PostDeploySummary(cfg *config.Config, result *postinstall.Result, steps []distribution.StepResult, runID string) string {
	clusterFQDN := cfg.Cluster.Name + "." + cfg.Cluster.Domain
	consoleURL := fmt.Sprintf("https://console-openshift-console.apps.%s", clusterFQDN)
	apiURL := fmt.Sprintf("https://api.%s:6443", clusterFQDN)

	sb := NewBuilder()
	sb.WriteString("\n")
	sb.WriteString("  " + tui.CompletionSuccess("cluster deployed") + "\n")
	sb.Newline()
	sb.KV("run_id", runID)
	sb.Newline()

	sb.Section("access")
	sb.KV("cluster", clusterFQDN)
	sb.KVWide("console", consoleURL)
	sb.KVWide("api", apiURL)
	sb.Newline()

	sb.Section("dns records")
	apiDomain := fmt.Sprintf("api.%s", clusterFQDN)
	appsDomain := fmt.Sprintf("*.apps.%s", clusterFQDN)
	if result != nil && result.DNSDeployed && result.KubeVipIP != "" {
		sb.KV(apiDomain, result.KubeVipIP+" (kube-vip)")
	} else if cfg.Networking.Bastion.IP != "" {
		sb.KV(apiDomain, cfg.Networking.Bastion.IP+" (haproxy)")
	}
	bastionIP := cfg.Networking.Bastion.IP
	if result != nil && result.BastionIP != "" {
		bastionIP = result.BastionIP
	}
	sb.KV(appsDomain, bastionIP+" (haproxy)")
	sb.Newline()

	sb.Section("status")
	if result != nil {
		if result.BootstrapCleaned {
			sb.KV("bootstrap", "cleaned up")
		} else {
			sb.KV("bootstrap", "still running")
		}
		if result.DNSDeployed && result.KubeVipIP != "" {
			sb.KV("api routing", fmt.Sprintf("kube-vip (%s)", result.KubeVipIP))
		} else {
			sb.KV("api routing", "haproxy (bastion)")
		}
		sb.KV("ingress routing", "haproxy (bastion)")
	}
	sb.Newline()

	if len(steps) > 0 {
		sb.Section("steps")
		var total time.Duration
		for _, s := range steps {
			total += s.Duration
			d := s.Duration.Truncate(time.Millisecond).String()
			sb.KV(string(s.StepID), fmt.Sprintf("%-*s  %s", stepStatusColWidth, displayStatus(&s), d))
		}
		sb.KV("total", total.Truncate(time.Millisecond).String())
		sb.Newline()
	}

	sb.Section("credentials")
	sb.KVHighlight("username", "kubeadmin")
	sb.KVWide("password", "cat okd-install/cluster-config/auth/kubeadmin-password")
	sb.Newline()

	sb.Section("quick start")
	sb.WriteString("    " + tui.CodeInlineStyle.Render("export KUBECONFIG=~/.kube/config") + "\n")
	sb.WriteString("    " + tui.CodeInlineStyle.Render("oc get nodes") + "\n")
	sb.Newline()

	sb.Section("next steps")
	sb.Para("cluster deployed with haproxy handling ingress on the bastion.")
	sb.Para("if you deploy a loadbalancer provider (e.g., metallb), run:")
	sb.WriteString("      " + tui.CodeInlineStyle.Render("okdctl update-ingress") + "\n")
	sb.Para("to auto-detect loadbalancer ips and switch dns over.")
	sb.Newline()

	return "\n" + tui.BoxedSectionCompact(sb.String(), "deployment complete", tui.DefaultBoxWidth) + "\n"
}

// InterruptSummary renders a partial-progress box for a Ctrl-C interruption;
// resumeCmd is the exact command the user should re-run.
func InterruptSummary(steps []distribution.StepResult, resumeCmd, runID string) string {
	sb := NewBuilder()
	sb.WriteString("\n")
	sb.WriteString("  " + tui.WarningStyle.Render("interrupted") + "\n")
	sb.Newline()
	sb.KV("run_id", runID)
	sb.Newline()

	if len(steps) > 0 {
		sb.Section("partial progress")
		for _, s := range steps {
			d := s.Duration.Truncate(time.Millisecond).String()
			sb.KV(string(s.StepID), fmt.Sprintf("%-*s  %s", stepStatusColWidth, displayStatus(&s), d))
		}
		sb.Newline()
	}

	sb.Section("resume")
	sb.WriteString("    " + tui.CodeInlineStyle.Render(resumeCmd) + "\n")
	sb.Newline()

	return "\n" + tui.BoxedSectionCompact(sb.String(), "interrupted", tui.DefaultBoxWidth) + "\n"
}

// FailureInfo describes a failed deploy for FailureSummary. Phase mirrors
// the on-disk marker phase so the resume line stays truthful.
type FailureInfo struct {
	Steps        []distribution.StepResult
	Phase        string
	RunID        string
	Elapsed      time.Duration
	TeardownCmd  string
	TeardownNote string
}

// FailureSummary renders the partial-progress box for a mid-phase deploy
// failure; next steps lead with resume, then --fresh, then teardown.
func FailureSummary(f *FailureInfo) string {
	sb := NewBuilder()
	sb.WriteString("\n")
	sb.WriteString("  " + tui.ErrorStyle.Render("deploy failed") + "\n")
	sb.Newline()
	sb.KV("run_id", f.RunID)
	sb.KV("failed phase", f.Phase)
	if step := failedStepID(f.Steps); step != "" {
		sb.KV("failed step", step)
	}
	sb.KV("elapsed", f.Elapsed.Truncate(time.Second).String())
	sb.Newline()

	if len(f.Steps) > 0 {
		sb.Section("partial progress")
		for _, s := range f.Steps {
			d := s.Duration.Truncate(time.Millisecond).String()
			sb.KV(string(s.StepID), fmt.Sprintf("%-*s  %s", stepStatusColWidth, displayStatus(&s), d))
		}
		sb.Newline()
	}

	sb.Section("next steps")
	sb.Bullet("re-run " + tui.CodeInlineStyle.Render("okdctl deploy") + " to resume from " + f.Phase)
	sb.Bullet("or " + tui.CodeInlineStyle.Render("okdctl deploy --fresh") + " to restart from scratch (wipes cluster credentials)")
	sb.Bullet("or " + tui.CodeInlineStyle.Render(f.TeardownCmd) + " to " + f.TeardownNote)
	sb.Newline()

	return "\n" + tui.BoxedSectionCompact(sb.String(), "deploy failed", tui.DefaultBoxWidth) + "\n"
}

func failedStepID(steps []distribution.StepResult) string {
	for i := len(steps) - 1; i >= 0; i-- {
		if !steps[i].Success && !steps[i].Skipped {
			return string(steps[i].StepID)
		}
	}
	return ""
}

// UpdateIngressSummary renders the update-ingress result: converted controllers
// and DNS record changes.
func UpdateIngressSummary(result *postinstall.UpdateIngressResult) string {
	sb := NewBuilder()
	sb.Newline()

	if result.ConvertedCount > 0 {
		sb.Section("conversion")
		sb.KV("controllers converted", fmt.Sprintf("%d (HostNetwork → LoadBalancerService)", result.ConvertedCount))
		sb.Newline()
	}

	sb.Section("dns records")
	if result.KubeVipIP != "" {
		sb.KVHighlight("api.*", result.KubeVipIP+" (kube-vip)")
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
		sb.KVHighlight(label, e.LBIP+suffix)
	}
	sb.Newline()

	sb.Section("status")
	if result.DNSReconciled {
		sb.KV("dns", "reconciled from bootstrap state")
	}
	if result.HAProxyRemoved {
		sb.KV("haproxy", "stopped and disabled")
	} else {
		sb.KV("haproxy", "still running")
	}
	sb.Newline()

	return "\n" + tui.BoxedSectionCompact(sb.String(), "ingress updated", tui.DefaultBoxWidth) + "\n"
}
