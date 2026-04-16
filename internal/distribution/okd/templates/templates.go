// Package templates provides embedded templates for OKD configuration generation.
package templates

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"text/template"
)

//go:embed *.tmpl
var templateFS embed.FS

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
	Architecture   string
}

type TerraformVarsData struct {
	ClusterName          string
	TargetNode           string
	Bridge               string
	OSStorage            string
	DataStorage          string
	FCOSISOStorage       string
	MasterISOsString     string
	WorkerISOsString     string
	VMIDBase             int
	MasterCount          int
	WorkerCount          int
	OSDiskSizeGB         int
	MasterOSDiskSizeGB   int
	WorkerOSDiskSizeGB   int
	WorkerDataDiskSizeGB int
	MasterDataDiskSizeGB int
	BootstrapCPUCores    int
	BootstrapMemoryMB    int
	MasterCPUCores       int
	MasterMemoryMB       int
	WorkerCPUCores       int
	WorkerMemoryMB       int
	MasterNames          string
	WorkerNames          string
	CPUType              string
	NUMAEnabled          bool
	AdditionalNetworks   string
	MasterTargetNodes    string
	WorkerTargetNodes    string
}

type HAProxyServer struct {
	Name string
	IP   string
}

type HAProxyConfigData struct {
	ClusterDomain string
	BootstrapIP   string
	MasterServers []HAProxyServer
	WorkerServers []HAProxyServer
	BackupServers []HAProxyServer // masters as backup for http/https when workers are configured
}

type DNSNode struct {
	Name string
	IP   string
}

// DNSCustomDomain maps a custom domain to a LoadBalancer IP.
type DNSCustomDomain struct {
	Domain string // e.g., "grappleberry.xyz"
	IP     string // LoadBalancer IP assigned by MetalLB
}

type DNSConfigData struct {
	ClusterName   string
	ClusterDomain string // e.g., "mycluster.k8s.local"

	// Load balancer IPs
	BastionIP string // HAProxy for API load balancing (bootstrap phase only)
	KubeVipIP string // kube-vip VIP for API (production only, takes over from HAProxy)
	AppsIP    string // Default router LB IP (auto-assigned by LB provider)

	// Custom domains served by non-default IngressControllers
	CustomDomains []DNSCustomDomain

	// Upstream DNS servers for forwarding external queries
	UpstreamDNS []string

	// Node details
	BootstrapNode DNSNode // only set in bootstrap config
	MasterNodes   []DNSNode
	WorkerNodes   []DNSNode
}

const DefaultKubeVIPImageTag = "v1.0.4"

type KubeVIPData struct {
	VIPAddress string // e.g. "192.168.227.10"
	Interface  string // interface used for ARP announcements, e.g. "ens18"
	ImageTag   string // e.g. "v1.0.4"
}

type PreInstallData struct {
	OSSerial   string
	DataSerial string
}

func RenderPreInstall(data PreInstallData) (string, error) {
	return renderTemplate("pre-install.sh.tmpl", data)
}

type CompactIngressData struct {
	Replicas int
}

func RenderCompactIngress(data CompactIngressData) (string, error) {
	return renderTemplate("ingress-controller-compact.yaml.tmpl", data)
}

func RenderInstallConfig(data *InstallConfigData) (string, error) {
	return renderTemplate("install-config.yaml.tmpl", data)
}

func RenderTerraformVars(data *TerraformVarsData) (string, error) {
	return renderTemplate("terraform.tfvars.tmpl", data)
}

func RenderHAProxyConfig(data *HAProxyConfigData) (string, error) {
	return renderTemplate("haproxy.cfg.tmpl", data)
}

func RenderDNSBootstrapConfig(data *DNSConfigData) (string, error) {
	return renderTemplate("dnsmasq-bootstrap.conf.tmpl", data)
}

func RenderDNSProductionConfig(data *DNSConfigData) (string, error) {
	return renderTemplate("dnsmasq-production.conf.tmpl", data)
}

type KubeVIPRBACManifest struct {
	Filename string
	Content  string
}

// Each resource is rendered separately because openshift-install only processes
// the first YAML document per file.
func RenderKubeVIPRBACManifests() ([]KubeVIPRBACManifest, error) {
	matches, err := fs.Glob(templateFS, "kube-vip-rbac-*.yaml.tmpl")
	if err != nil {
		return nil, err
	}

	var manifests []KubeVIPRBACManifest
	for _, tmpl := range matches {
		content, err := renderTemplate(tmpl, nil)
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, KubeVIPRBACManifest{
			Filename: "99-" + strings.TrimSuffix(tmpl, ".tmpl"),
			Content:  content,
		})
	}
	return manifests, nil
}

func RenderKubeVIPDaemonSet(data KubeVIPData) (string, error) {
	if data.ImageTag == "" {
		data.ImageTag = DefaultKubeVIPImageTag
	}
	return renderTemplate("kube-vip-daemonset.yaml.tmpl", data)
}

var templateFuncs = template.FuncMap{
	"split":      strings.Split,
	"trimPrefix": strings.TrimPrefix,
	"trimSuffix": strings.TrimSuffix,
	"join":       strings.Join,
	"reversePTR": reversePTR,
}

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

func renderTemplate(name string, data any) (string, error) {
	content, err := templateFS.ReadFile(name)
	if err != nil {
		return "", fmt.Errorf("failed to read template %s: %w", name, err)
	}

	tmpl, err := template.New(name).Funcs(templateFuncs).Parse(string(content))
	if err != nil {
		return "", fmt.Errorf("failed to parse template %s: %w", name, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template %s: %w", name, err)
	}

	return buf.String(), nil
}
