package okd

import "github.com/qxtaiba/okd-proxmox-cli/internal/distribution"

type ClusterStatus struct {
	Phase        ClusterPhase
	Version      string
	APIServerURL string
	ConsoleURL   string
	Nodes        []NodeStatus
	Conditions   []Condition
	Message      string
}

type ClusterPhase string

const (
	PhasePending    ClusterPhase = "Pending"
	PhaseInstalling ClusterPhase = "Installing"
	PhaseRunning    ClusterPhase = "Running"
	PhaseDegraded   ClusterPhase = "Degraded"
	PhaseFailed     ClusterPhase = "Failed"
	PhaseUnknown    ClusterPhase = "Unknown"
)

type NodeStatus struct {
	Name       string
	Role       string
	Status     string
	Version    string
	InternalIP string
	Conditions []Condition
}

type Condition struct {
	Type    string
	Status  string
	Reason  string
	Message string
}

const (
	StepPreflight         distribution.StepID = "preflight"
	StepInstallPackages   distribution.StepID = "install-packages"
	StepCleanup           distribution.StepID = "cleanup"
	StepEnsureWorkDir     distribution.StepID = "ensure-workdir"
	StepDownloadTools     distribution.StepID = "download-tools"
	StepGenerateConfig    distribution.StepID = "generate-config"
	StepGenerateManifests distribution.StepID = "generate-manifests"
	StepInjectManifests   distribution.StepID = "inject-manifests"
	StepGenerateIgnition  distribution.StepID = "generate-ignition"
	StepInstallApache     distribution.StepID = "install-apache"
	StepDeployIgnition    distribution.StepID = "deploy-ignition"
	StepVerifyWebServer   distribution.StepID = "verify-webserver"
	StepBuildISOs         distribution.StepID = "build-isos"
	StepUploadISOs        distribution.StepID = "upload-isos"
	StepGenerateTfvars    distribution.StepID = "generate-tfvars"
	StepConfigureHAProxy  distribution.StepID = "configure-haproxy"
	StepConfigureFirewall distribution.StepID = "configure-firewall"
	StepGenerateDNS       distribution.StepID = "generate-dns"

	StepDeployInfra     distribution.StepID = "deploy-infrastructure"
	StepWaitBootstrap   distribution.StepID = "wait-bootstrap"
	StepSetupKubeconfig distribution.StepID = "setup-kubeconfig"
	StepValidateAccess  distribution.StepID = "validate-access"
	StepMonitorInstall  distribution.StepID = "monitor-install"
	StepSetupAccess     distribution.StepID = "setup-access"
)
