package steps

import (
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

func TestFormatShapedFields_HelpStatesShape(t *testing.T) {
	cases := []struct {
		step  *wizard.StepDefinition
		key   string
		token string
	}{
		{&BasicsStepDefinition, config.FieldClusterDomain, "example.com"},
		{&ProxmoxStepDefinition, config.FieldProxmoxEndpoint, "https://192.168.1.100:8006"},
		{&ProxmoxStepDefinition, fieldAPIToken, "user@realm!tokenname=secret"},
		{&ProxmoxStepDefinition, fieldAPIToken, "PROXMOX_VE_API_TOKEN"},
		{&ProxmoxStepDefinition, fieldInsecure, "PROXMOX_VE_INSECURE"},
		{&ProxmoxStepDefinition, config.FieldProxmoxNodes, "pve1, pve2"},
		{&ProxmoxStepDefinition, config.FieldProxmoxVLAN, "0 is untagged"},
		{&ProxmoxStepDefinition, config.FieldProxmoxVMIDBase, "6000"},
		{&NetworkingStepDefinition, config.FieldNetworkingMachineCIDR, "192.168.1.0/24"},
		{&NetworkingStepDefinition, config.FieldNetworkingGateway, "ip address"},
		{&NetworkingStepDefinition, config.FieldNetworkingDNS, "comma-separated resolver ip addresses"},
		{&NetworkingStepDefinition, config.FieldNetworkingAPIVIP, "192.168.1.158"},
		{&NetworkingStepDefinition, config.FieldNetworkingIngressVIP, "192.168.1.159"},
		{&NetworkingStepDefinition, config.FieldNetworkingNodeIPStart, "192.168.1.150"},
		{&NetworkingStepDefinition, fieldNTP, "comma-separated hostnames or ips"},
		{&AdvancedStepDefinition, config.FieldNetworkingPodCIDR, "10.128.0.0/14"},
		{&AdvancedStepDefinition, config.FieldNetworkingServiceCIDR, "172.30.0.0/16"},
		{&AdvancedStepDefinition, fieldHookTimeout, "15m"},
		{&AdvancedStepDefinition, fieldDrainTimeout, "10m"},
		{&FilesStepDefinition, config.FieldFilesPullSecret, "path to"},
		{&FilesStepDefinition, config.FieldFilesSSHPublicKey, "path to"},
		{&ResourcesStepDefinition, fieldCPCPU, "okd minimum: 4 vcpus"},
		{&ResourcesStepDefinition, fieldCPMemory, "okd minimum: 16384 mib (16 gib)"},
		{&ResourcesStepDefinition, fieldWorkerCPU, "okd minimum: 2 vcpus"},
		{&ResourcesStepDefinition, fieldWorkerMemory, "okd minimum: 8192 mib (8 gib)"},
	}
	for _, tc := range cases {
		fd := findField(t, tc.step, tc.key)
		if !strings.Contains(fd.Help, tc.token) {
			t.Errorf("%s.%s: Help %q does not state the shape token %q", tc.step.ID, tc.key, fd.Help, tc.token)
		}
	}
}

func TestProxmoxStep_TokenHelpNamesTheRealEnvVar(t *testing.T) {
	fd := findField(t, &ProxmoxStepDefinition, fieldAPIToken)
	if strings.Contains(fd.Help, "PROXMOX_VE_API_TOKEN_SECRET") {
		t.Errorf("api token Help %q names PROXMOX_VE_API_TOKEN_SECRET, which does not exist", fd.Help)
	}
	if fd.ConfigSet != nil || fd.ConfigGet != nil {
		t.Error("the api token field must never read or write the config")
	}
}
