package steps

import "github.com/qxtaiba/okdctl/internal/config"

// Field keys mirror okdctl.yaml json paths; api_token and insecure are the
// env-only exceptions the wizard writes to okdctl.env, never to the yaml.
const (
	fieldAPIToken       = "provider.proxmox.api_token" //nolint:gosec // G101: a field key, not a credential
	fieldInsecure       = "provider.proxmox.insecure"
	fieldPool           = "provider.proxmox.pool"
	fieldHA             = "provider.proxmox.ha"
	fieldNTP            = "networking.ntp"
	fieldCPCPU          = config.FieldControlPlane + ".cpu"
	fieldCPMemory       = config.FieldControlPlane + ".memory_mib"
	fieldCPRootDisk     = config.FieldControlPlane + ".disks[0].size_gib"
	fieldWorkerCount    = config.FieldWorkers + ".count"
	fieldWorkerCPU      = config.FieldWorkers + ".cpu"
	fieldWorkerMemory   = config.FieldWorkers + ".memory_mib"
	fieldWorkerRootDisk = config.FieldWorkers + ".disks[0].size_gib"
	fieldWorkerDataDisk = config.FieldWorkers + ".disks[1].size_gib"
	fieldHookCommand    = "lifecycle.before_next_node.command"
	fieldHookTimeout    = "lifecycle.before_next_node.timeout"
	fieldDrainTimeout   = "lifecycle.drain_timeout"
	fieldHandoff        = "handoff.enabled"
	fieldCatalogs       = "defaults.disable_redhat_catalogs"
)

const (
	labelDeploy           = "deploy"
	labelCluster          = "cluster"
	labelCPU              = "cpu"
	labelVLAN             = "vlan"
	labelCPUType          = "cpu type"
	labelVMIDBase         = "vm id base"
	labelMemory           = "memory (mib)"
	labelRootDisk         = "root disk (gib)"
	roleLabelControlPlane = "control plane"
	roleLabelWorkers      = "workers"
	labelSelectedCapacity = "selected capacity"
	statusUnavailable     = "unavailable"
	statusNotChecked      = "not checked"
	valYes                = "yes"
	valNo                 = "no"
)
