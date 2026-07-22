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

// InstallConfigData is the template binding for install-config.yaml.
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

// TerraformVarsData is the template binding for terraform.tfvars.
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
	HAEnabled            bool
	AdditionalNetworks   string
	MasterTargetNodes    string
	WorkerTargetNodes    string
}

// HAProxyServer is a single backend entry in the HAProxy config template.
type HAProxyServer struct {
	Name string
	IP   string
}

// HAProxyConfigData is the template binding for haproxy.cfg.
type HAProxyConfigData struct {
	ClusterDomain string
	BootstrapIP   string
	MasterServers []HAProxyServer
	WorkerServers []HAProxyServer
	BackupServers []HAProxyServer // masters as backup for http/https when workers are configured
}

// DNSNode is one A/PTR record pair in the dnsmasq config template.
type DNSNode struct {
	Name string
	IP   string
}

// DNSCustomDomain maps a custom domain to a LoadBalancer IP.
type DNSCustomDomain struct {
	Domain string // e.g., "grappleberry.xyz"
	IP     string // LoadBalancer IP assigned by MetalLB
}

// DNSConfigData is the template binding for dnsmasq bootstrap and production
// configs.
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

// DefaultKubeVIPImageTag is the kube-vip image tag used when KubeVIPData
// leaves ImageTag empty.
const DefaultKubeVIPImageTag = "v1.0.4"

// KubeVIPData is the template binding for the kube-vip DaemonSet manifest.
type KubeVIPData struct {
	VIPAddress string // e.g. "192.168.227.10"
	Interface  string // interface used for ARP announcements, e.g. "ens18"
	ImageTag   string // e.g. "v1.0.4"
}

// PreInstallData is the template binding for the pre-install shell script.
type PreInstallData struct {
	OSSerial   string
	DataSerial string
}

// RenderPreInstall renders the pre-install shell script from data.
func RenderPreInstall(data PreInstallData) (string, error) {
	return renderTemplate("pre-install.sh.tmpl", data)
}

// CompactIngressData is the template binding for the compact-mode ingress
// controller manifest.
type CompactIngressData struct {
	Replicas int
}

// RenderCompactIngress renders the compact-mode ingress controller manifest.
func RenderCompactIngress(data CompactIngressData) (string, error) {
	return renderTemplate("ingress-controller-compact.yaml.tmpl", data)
}

// RenderInstallConfig renders install-config.yaml from data.
func RenderInstallConfig(data *InstallConfigData) (string, error) {
	return renderTemplate("install-config.yaml.tmpl", data)
}

// RenderTerraformVars renders terraform.tfvars from data.
func RenderTerraformVars(data *TerraformVarsData) (string, error) {
	return renderTemplate("terraform.tfvars.tmpl", data)
}

// RenderHAProxyConfig renders haproxy.cfg from data.
func RenderHAProxyConfig(data *HAProxyConfigData) (string, error) {
	return renderTemplate("haproxy.cfg.tmpl", data)
}

// RenderDNSBootstrapConfig renders the dnsmasq config used during bootstrap.
func RenderDNSBootstrapConfig(data *DNSConfigData) (string, error) {
	return renderTemplate("dnsmasq-bootstrap.conf.tmpl", data)
}

// RenderDNSProductionConfig renders the dnsmasq config used once the cluster
// is live.
func RenderDNSProductionConfig(data *DNSConfigData) (string, error) {
	return renderTemplate("dnsmasq-production.conf.tmpl", data)
}

// KubeVIPRBACManifest is one rendered kube-vip RBAC YAML document with its
// target filename.
type KubeVIPRBACManifest struct {
	Filename string
	Content  string
}

// RenderKubeVIPRBACManifests renders each kube-vip RBAC manifest as a
// separate document. Each resource is rendered separately because
// openshift-install only processes the first YAML document per file.
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

// RenderKubeVIPDaemonSet renders the kube-vip DaemonSet manifest from data,
// defaulting ImageTag to DefaultKubeVIPImageTag when empty.
func RenderKubeVIPDaemonSet(data KubeVIPData) (string, error) {
	if data.ImageTag == "" {
		data.ImageTag = DefaultKubeVIPImageTag
	}
	return renderTemplate("kube-vip-daemonset.yaml.tmpl", data)
}

// ChronyConfData is the template binding for the chrony.conf shipped to
// every master/worker node via MachineConfig.
type ChronyConfData struct {
	Server string
}

// RenderChronyConf renders /etc/chrony.conf pointed at data.Server. It
// always steps the clock unconditionally (makestep 1.0 -1) rather than
// slewing: Proxmox pause/resume can jump the guest clock by minutes in one
// shot, and chrony's slew-only convergence takes hours — long enough for
// etcd leader elections to fail and node certs to read "not yet valid" in
// the meantime.
func RenderChronyConf(data ChronyConfData) (string, error) {
	return renderTemplate("chrony.conf.tmpl", data)
}

// ChronyMachineConfigData is the template binding for the chrony
// MachineConfig manifest, rendered once per node pool (master, worker).
type ChronyMachineConfigData struct {
	Role   string // machineconfiguration.openshift.io/role label value
	Name   string // metadata.name
	Source string // ignition storage.files contents.source data URL
}

// RenderChronyMachineConfig renders the chrony MachineConfig manifest for
// data.Role from data.Source.
func RenderChronyMachineConfig(data ChronyMachineConfigData) (string, error) {
	return renderTemplate("chrony-machineconfig.yaml.tmpl", data)
}

// FstrimMachineConfigData is the template binding for the fstrim
// MachineConfig manifest, rendered once per node pool (master, worker).
type FstrimMachineConfigData struct {
	Role string // machineconfiguration.openshift.io/role label value
	Name string // metadata.name
}

// RenderFstrimMachineConfig renders the fstrim MachineConfig manifest for
// data.Role. It masks FCOS's stock fstrim.timer (which fails because FCOS
// ships no /etc/fstab for `fstrim --fstab` to read, see
// coreos/fedora-coreos-tracker#468) and ships a replacement unit trimming
// explicit mountpoints.
func RenderFstrimMachineConfig(data FstrimMachineConfigData) (string, error) {
	return renderTemplate("fstrim-machineconfig.yaml.tmpl", data)
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
		return "", fmt.Errorf("read template %s: %w", name, err)
	}

	tmpl, err := template.New(name).Funcs(templateFuncs).Parse(string(content))
	if err != nil {
		return "", fmt.Errorf("parse template %s: %w", name, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template %s: %w", name, err)
	}

	return buf.String(), nil
}
