// Package templates provides embedded templates for OKD configuration generation.
package templates

import (
	"bytes"
	"embed"
	"strconv"
	"strings"
	"text/template"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

//go:embed *.tmpl
var templateFS embed.FS

// InstallConfigData holds data for install-config.yaml generation.
type InstallConfigData struct {
	ClusterName    string
	BaseDomain     string
	MasterReplicas int
	WorkerReplicas int
	ClusterCIDR    string
	HostPrefix     int
	MachineCIDR    string
	ServiceCIDR    string
	PullSecret     string
	SSHKey         string
}

// TerraformVarsData holds data for terraform.tfvars generation.
type TerraformVarsData struct {
	ClusterName       string
	TargetNode        string
	Bridge            string
	OSStorage         string
	DataStorage       string
	FCOSISOStorage    string
	MasterISOsString  string
	WorkerISOsString  string
	VMIDBase          int
	MasterCount       int
	WorkerCount       int
	OSDiskSizeGB      int
	DataDiskSizeGB    int
	BootstrapCPUCores int
	BootstrapMemoryMB int
	MasterCPUCores    int
	MasterMemoryMB    int
	WorkerCPUCores    int
	WorkerMemoryMB    int
	MasterNames       string
	WorkerNames       string
}

// HAProxyServer represents a server entry in HAProxy configuration.
type HAProxyServer struct {
	Name string
	IP   string
}

// HAProxyConfigData holds data for haproxy.cfg generation.
type HAProxyConfigData struct {
	ClusterDomain string
	BootstrapIP   string
	MasterServers []HAProxyServer
	WorkerServers []HAProxyServer
}

// DNSNode represents a cluster node for DNS configuration.
type DNSNode struct {
	Name string
	IP   string
}

// DNSConfigData holds data for dnsmasq configuration generation.
type DNSConfigData struct {
	ClusterName   string
	ClusterDomain string // e.g., "grappleberry.k8s.local"

	// Load balancer IPs
	BastionIP   string // HAProxy for API load balancing (bootstrap phase only)
	KubeVipIP   string // kube-vip VIP for API (production only, takes over from HAProxy)
	AppsIP         string // Default router LB IP (auto-assigned by MetalLB)
	UserAppsIP     string // User apps router IP (optional, for custom domain)
	UserAppsDomain string // User apps domain (e.g., "grappleberry.xyz")

	// Upstream DNS servers for forwarding external queries
	UpstreamDNS []string

	// Node details
	BootstrapNode DNSNode   // Bootstrap node (only in bootstrap config)
	MasterNodes   []DNSNode // Control plane nodes
	WorkerNodes   []DNSNode // Worker nodes
}

// RenderInstallConfig generates install-config.yaml from template.
func RenderInstallConfig(data InstallConfigData) (string, error) {
	return renderTemplate("install-config.yaml.tmpl", data)
}

// RenderTerraformVars generates terraform.tfvars from template.
func RenderTerraformVars(data TerraformVarsData) (string, error) {
	return renderTemplate("terraform.tfvars.tmpl", data)
}

// RenderHAProxyConfig generates haproxy.cfg from template.
func RenderHAProxyConfig(data HAProxyConfigData) (string, error) {
	return renderTemplate("haproxy.cfg.tmpl", data)
}

// RenderDNSBootstrapConfig generates dnsmasq-bootstrap.conf from template.
func RenderDNSBootstrapConfig(data DNSConfigData) (string, error) {
	return renderTemplate("dnsmasq-bootstrap.conf.tmpl", data)
}

// RenderDNSProductionConfig generates dnsmasq-production.conf from template.
func RenderDNSProductionConfig(data DNSConfigData) (string, error) {
	return renderTemplate("dnsmasq-production.conf.tmpl", data)
}

// templateFuncs provides custom functions for templates.
var templateFuncs = template.FuncMap{
	"split":      strings.Split,
	"trimPrefix": strings.TrimPrefix,
	"trimSuffix": strings.TrimSuffix,
	"join":       strings.Join,
	"reversePTR": reversePTR,
}

// reversePTR safely reverses an IPv4 address for PTR record generation.
// Returns the reversed octets (e.g., "192.168.1.10" -> "10.1.168.192").
// Returns empty string if the IP is invalid or empty.
func reversePTR(ip string) string {
	if ip == "" {
		return ""
	}
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return ""
	}
	// Validate each part is a valid IPv4 octet (0-255)
	for _, part := range parts {
		if part == "" {
			return ""
		}
		val, err := strconv.Atoi(part)
		if err != nil || val < 0 || val > 255 {
			return ""
		}
	}
	return parts[3] + "." + parts[2] + "." + parts[1] + "." + parts[0]
}

func renderTemplate(name string, data interface{}) (string, error) {
	content, err := templateFS.ReadFile(name)
	if err != nil {
		return "", utils.WrapErrorf(err, "failed to read template %s", name)
	}

	tmpl, err := template.New(name).Funcs(templateFuncs).Parse(string(content))
	if err != nil {
		return "", utils.WrapErrorf(err, "failed to parse template %s", name)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", utils.WrapErrorf(err, "failed to execute template %s", name)
	}

	return buf.String(), nil
}
