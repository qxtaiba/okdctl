package cli

import (
	"fmt"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/deployment"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/netutil"
)

const defaultContentWidth = tui.DefaultBoxWidth - 2
const defaultKeyColWidth  = 45

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
			sb.WriteString("  " + tui.ErrorStyle.Render(tui.IconError) + " ")
			if e.Field != "" {
				sb.WriteString(tui.MutedStyle.Render(e.Field + ": "))
			}
			sb.WriteString(e.Message + "\n")
		}
	}

	return sb.String()
}

func DeploySummary(cfg *config.Config) string {
	title := fmt.Sprintf("%s CLUSTER SETUP", strings.ToUpper(string(cfg.Distribution.Type)))
	clusterDomain := fmt.Sprintf("%s.%s", cfg.Cluster.Name, cfg.Cluster.Domain)

	sb := newSummaryBuilder()
	sb.newline()

	sb.section("cluster")
	sb.kvHighlight("name", cfg.Cluster.Name)
	sb.kv("base domain", cfg.Cluster.Domain)
	sb.kvHighlight("full domain", clusterDomain)
	sb.kvHighlight("version", cfg.Distribution.Version)
	sb.newline()

	sb.section("control plane")
	sb.kvHighlight("nodes", fmt.Sprintf("%d", cfg.Topology.ControlPlane.Count))
	sb.kv("cpu", fmt.Sprintf("%dvCPU/node", cfg.Topology.ControlPlane.CPU))
	sb.kv("memory", fmt.Sprintf("%dGB/node", cfg.Topology.ControlPlane.Memory/1024))
	sb.kv("disk", fmt.Sprintf("%dGB/node", cfg.Topology.ControlPlane.Disk))
	sb.newline()

	sb.section("workers")
	sb.kvHighlight("nodes", fmt.Sprintf("%d", cfg.Topology.Workers.Count))
	sb.kv("cpu", fmt.Sprintf("%dvCPU/node", cfg.Topology.Workers.CPU))
	sb.kv("memory", fmt.Sprintf("%dGB/node", cfg.Topology.Workers.Memory/1024))
	sb.kv("disk", fmt.Sprintf("%dGB/node", cfg.Topology.Workers.Disk))
	sb.newline()

	sb.section("network")
	sb.kv("machine cidr", cfg.Networking.MachineCIDR)
	sb.kv("pod cidr", cfg.Networking.PodCIDR)
	sb.kv("service cidr", cfg.Networking.ServiceCIDR)

	if cfg.Networking.StaticIP.Start != "" {
		if vip := netutil.DeriveVIPFromStaticIP(cfg.Networking.StaticIP.Start); vip != "" {
			sb.kvHighlight("vip address", vip)
		}
	}
	sb.newline()

	return "\n" + tui.BoxedSectionCompact(sb.String(), title, tui.DefaultBoxWidth) + "\n"
}

func PostDeploySummary(cfg *config.Config, result *deployment.Result) string {
	var sb strings.Builder

	clusterFQDN := cfg.Cluster.Name + "." + cfg.Cluster.Domain
	consoleURL := fmt.Sprintf("https://console-openshift-console.apps.%s", clusterFQDN)
	apiURL := fmt.Sprintf("https://api.%s:6443", clusterFQDN)

	kvWidth := defaultContentWidth - 2
	var content strings.Builder

	content.WriteString("\n")
	content.WriteString("  " + tui.CompletionSuccess("cluster deployed successfully!") + "\n\n")
	content.WriteString("  " + tui.SubsectionLabel("access") + "\n")
	content.WriteString("  " + tui.DottedKeyValueFull("  cluster", clusterFQDN, defaultKeyColWidth, kvWidth) + "\n")
	content.WriteString("  " + tui.DottedKeyValueFull("  console", consoleURL, defaultKeyColWidth, kvWidth) + "\n")
	content.WriteString("  " + tui.DottedKeyValueFull("  api", apiURL, defaultKeyColWidth, kvWidth) + "\n")
	content.WriteString("\n")

	content.WriteString("  " + tui.SubsectionLabel("dns records") + "\n")
	apiDomain := fmt.Sprintf("api.%s", clusterFQDN)
	appsDomain := fmt.Sprintf("*.apps.%s", clusterFQDN)
	if result != nil && result.APIDNSSwitched && result.KubeVipIP != "" {
		content.WriteString("  " + tui.DottedKeyValueFull("  "+apiDomain, result.KubeVipIP+" (kube-vip)", defaultKeyColWidth, kvWidth) + "\n")
	} else if bastionIP := cfg.Networking.Bastion.IP; bastionIP != "" {
		content.WriteString("  " + tui.DottedKeyValueFull("  "+apiDomain, bastionIP+" (haproxy)", defaultKeyColWidth, kvWidth) + "\n")
	}
	if result != nil && result.RouterLBIP != "" {
		content.WriteString("  " + tui.DottedKeyValueFull("  "+appsDomain, result.RouterLBIP, defaultKeyColWidth, kvWidth) + "\n")
	}
	if result != nil && result.CustomRouterIP != "" {
		customLabel := fmt.Sprintf("*.%s", cfg.Networking.CustomDomain)
		if cfg.Networking.CustomDomain == "" {
			customLabel = fmt.Sprintf("router-%s (custom)", cfg.Cluster.Name)
		}
		content.WriteString("  " + tui.DottedKeyValueFull("  "+customLabel, result.CustomRouterIP, defaultKeyColWidth, kvWidth) + "\n")
	}
	content.WriteString("\n")

	content.WriteString("  " + tui.SubsectionLabel("status") + "\n")
	if result != nil {
		if result.BootstrapCleaned {
			content.WriteString("  " + tui.DottedKeyValueFull("  bootstrap", "cleaned up", defaultKeyColWidth, kvWidth) + "\n")
		} else {
			content.WriteString("  " + tui.DottedKeyValueFull("  bootstrap", "still running", defaultKeyColWidth, kvWidth) + "\n")
		}
		if result.APIDNSSwitched && result.KubeVipIP != "" {
			content.WriteString("  " + tui.DottedKeyValueFull("  api routing", fmt.Sprintf("kube-vip (%s)", result.KubeVipIP), defaultKeyColWidth, kvWidth) + "\n")
		} else {
			content.WriteString("  " + tui.DottedKeyValueFull("  api routing", "haproxy (bastion)", defaultKeyColWidth, kvWidth) + "\n")
		}
	}
	content.WriteString("\n")

	content.WriteString("  " + tui.SubsectionLabel("credentials") + "\n")
	content.WriteString("  " + tui.DottedKeyValueHighlightFull("  username", "kubeadmin", defaultKeyColWidth, kvWidth) + "\n")
	content.WriteString("  " + tui.DottedKeyValueFull("  password", "cat okd-install/cluster-config/auth/kubeadmin-password", defaultKeyColWidth, kvWidth) + "\n")
	content.WriteString("\n")

	content.WriteString("  " + tui.SubsectionLabel("quick start") + "\n")
	content.WriteString("    " + tui.CodeInlineStyle.Render("export KUBECONFIG=~/.kube/config") + "\n")
	content.WriteString("    " + tui.CodeInlineStyle.Render("oc get nodes") + "\n")

	content.WriteString("\n")
	sb.WriteString("\n")
	sb.WriteString(tui.BoxedSectionCompact(content.String(), "DEPLOYMENT COMPLETE", tui.DefaultBoxWidth))
	sb.WriteString("\n")

	return sb.String()
}

