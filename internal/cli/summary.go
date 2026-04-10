package cli

import (
	"fmt"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/postinstall"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui"
)

const (
	defaultContentWidth = tui.DefaultBoxWidth - 2
	defaultKeyColWidth  = 45
)

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

func PostDeploySummary(cfg *config.Config, result *postinstall.Result) string {
	clusterFQDN := cfg.Cluster.Name + "." + cfg.Cluster.Domain
	consoleURL := fmt.Sprintf("https://console-openshift-console.apps.%s", clusterFQDN)
	apiURL := fmt.Sprintf("https://api.%s:6443", clusterFQDN)

	sb := newSummaryBuilder()
	sb.b.WriteString("\n")
	sb.b.WriteString("  " + tui.CompletionSuccess("cluster deployed successfully!") + "\n")
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
	sb.b.WriteString("      " + tui.CodeInlineStyle.Render("openshitctl update-ingress") + "\n")
	sb.b.WriteString("    to auto-detect loadbalancer ips and switch dns over.\n")
	sb.newline()

	return "\n" + tui.BoxedSectionCompact(sb.String(), "deployment complete", tui.DefaultBoxWidth) + "\n"
}

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
