package deployexec

import (
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/install"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/postinstall"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/setup"
)

// Phase names one group of the deploy stream's checklist — a coarser unit than
// the engine's own setup/install/postinstall phases, which put unrelated work
// (manifest generation and terraform apply) under one label.
type Phase string

// The five phases the checklist groups the step registry into.
const (
	PhasePrep     Phase = "prep"
	PhaseIgnition Phase = "ignition"
	PhaseInfra    Phase = "infra"
	PhaseInstall  Phase = "install"
	PhaseVerify   Phase = "verify"
)

// PhaseOrder lists the phases in the order the engine runs them, which is the
// order the checklist renders and collapses them in.
func PhaseOrder() []Phase {
	return []Phase{PhasePrep, PhaseIgnition, PhaseInfra, PhaseInstall, PhaseVerify}
}

// stepPhases maps every registered deploy step onto the phase that owns it.
// TestDeployPhases_CoverEveryRegisteredStepExactlyOnce proves this map and the
// engine's step registry never drift apart.
var stepPhases = map[distribution.StepID]Phase{
	// prep: bastion packages and tools, then the cluster's own definition.
	setup.StepInstallPackages:   PhasePrep,
	setup.StepInstallTools:      PhasePrep,
	setup.StepEnsureWorkDir:     PhasePrep,
	setup.StepDownloadTools:     PhasePrep,
	setup.StepGenerateConfig:    PhasePrep,
	setup.StepGenerateManifests: PhasePrep,
	setup.StepGenerateKubeVIP:   PhasePrep,
	setup.StepGenerateChrony:    PhasePrep,
	setup.StepGenerateFstrim:    PhasePrep,
	setup.StepInjectManifests:   PhasePrep,
	setup.StepCompactCluster:    PhasePrep,

	// ignition: the boot assets and everything that serves them to a node.
	setup.StepGenerateIgnition: PhaseIgnition,
	setup.StepInstallApache:    PhaseIgnition,
	setup.StepDeployIgnition:   PhaseIgnition,
	setup.StepVerifyWebServer:  PhaseIgnition,
	setup.StepBuildISOs:        PhaseIgnition,
	setup.StepUploadISOs:       PhaseIgnition,

	// infra: the bastion's network plumbing, then terraform bringing the vms up.
	setup.StepGenerateTfvars:    PhaseInfra,
	setup.StepConfigureHAProxy:  PhaseInfra,
	setup.StepConfigureFirewall: PhaseInfra,
	setup.StepConfigureDNS:      PhaseInfra,
	install.StepDeployInfra:     PhaseInfra,

	// install: bootstrap through a cluster that answers oc.
	install.StepWaitBootstrap:   PhaseInstall,
	install.StepStartWorkers:    PhaseInstall,
	install.StepSetupKubeconfig: PhaseInstall,
	install.StepValidateAccess:  PhaseInstall,
	install.StepMonitorInstall:  PhaseInstall,
	install.StepSetupAccess:     PhaseInstall,

	// verify: health gates, bootstrap teardown, and the day-1 extras.
	postinstall.StepVerifyHealth:        PhaseVerify,
	postinstall.StepVerifyKubeVIP:       PhaseVerify,
	postinstall.StepCleanupBootstrap:    PhaseVerify,
	postinstall.StepStopIgnitionServer:  PhaseVerify,
	postinstall.StepDeployProductionDNS: PhaseVerify,
	postinstall.StepInstallAddons:       PhaseVerify,
	postinstall.StepDisableRHDefaults:   PhaseVerify,
}

// PhaseOf returns the phase that owns id. A step no phase claims reports
// false, which the coverage test forbids — the screen then shows it under its
// engine phase rather than dropping it.
func PhaseOf(id distribution.StepID) (Phase, bool) {
	p, ok := stepPhases[id]
	return p, ok
}
