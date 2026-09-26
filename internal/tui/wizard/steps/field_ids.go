package steps

// Shared field/label IDs — keeps goconst quiet and avoids drift across sites.
const (
	fieldHost                 = "host"
	fieldDomain               = "domain"
	fieldGateway              = "gateway"
	fieldInterface            = "interface"
	fieldBridge               = "bridge"
	fieldDataStorage          = "data storage"
	fieldUsername             = "username"
	labelCluster              = "cluster"
	labelVMIDBase             = "vm id base"
	labelCPUType              = "cpu type"
	labelNTPServer            = "ntp server"
	labelTerraformEnvironment = "terraform environment"
	labelAutoApprove          = "auto approve"
	labelMachineCIDR          = "machine cidr"
	labelUpstreamDNS          = "upstream dns"
	labelPodCIDR              = "pod cidr"
	labelServiceCIDR          = "service cidr"
	labelHostPrefix           = "host prefix"
	labelAPIVIP               = "api vip"
	labelTokenID              = "token id"
	labelDeploy               = "deploy"
	statusUnavailable         = "unavailable"
	statusNotChecked          = "not checked"
	labelSelectedCapacity     = "selected capacity"
)
