package steps

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"sigs.k8s.io/yaml"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

func findField(t *testing.T, def *wizard.StepDefinition, key string) wizard.FieldDefinition {
	t.Helper()
	for i := range def.Sections {
		fields := def.Sections[i].Fields
		for j := range fields {
			if fields[j].Key == key {
				return fields[j]
			}
		}
	}
	t.Fatalf("field %q not found in step %q", key, def.ID)
	return wizard.FieldDefinition{}
}

func TestStepDefinitionsRoundTripTheirFields(t *testing.T) {
	cases := []struct {
		def        *wizard.StepDefinition
		key, value string
	}{
		{&BasicsStepDefinition, config.FieldClusterName, "homelab"},
		{&BasicsStepDefinition, config.FieldClusterDomain, "prod.example"},
		{&ProxmoxStepDefinition, config.FieldProxmoxEndpoint, "https://10.0.0.5:8006"},
		{&ProxmoxStepDefinition, config.FieldProxmoxNodes, "pve1, pve2"},
		{&ProxmoxStepDefinition, config.FieldProxmoxStorage, "tank"},
		{&ProxmoxStepDefinition, config.FieldProxmoxISOStorage, "isos"},
		{&ProxmoxStepDefinition, config.FieldProxmoxBridge, "vmbr1"},
		{&ProxmoxStepDefinition, config.FieldProxmoxVLAN, "42"},
		{&ProxmoxStepDefinition, config.FieldProxmoxCPUType, "x86-64-v3"},
		{&ProxmoxStepDefinition, fieldPool, "okd"},
		{&ProxmoxStepDefinition, fieldHA, valYes},
		{&ProxmoxStepDefinition, config.FieldProxmoxVMIDBase, "7000"},
		{&NetworkingStepDefinition, config.FieldNetworkingMachineCIDR, "10.0.0.0/24"},
		{&NetworkingStepDefinition, config.FieldNetworkingGateway, "10.0.0.1"},
		{&NetworkingStepDefinition, config.FieldNetworkingDNS, "10.0.0.1, 8.8.8.8"},
		{&NetworkingStepDefinition, config.FieldNetworkingAPIVIP, "10.0.0.98"},
		{&NetworkingStepDefinition, config.FieldNetworkingIngressVIP, "10.0.0.99"},
		{&NetworkingStepDefinition, config.FieldNetworkingNodeIPStart, "10.0.0.50"},
		{&NetworkingStepDefinition, fieldNTP, "time.example, 10.0.0.2"},
		{&ResourcesStepDefinition, fieldCPCPU, "6"},
		{&ResourcesStepDefinition, fieldCPMemory, "32768"},
		{&ResourcesStepDefinition, fieldCPRootDisk, "160"},
		{&ResourcesStepDefinition, fieldWorkerCount, "5"},
		{&ResourcesStepDefinition, fieldWorkerCPU, "12"},
		{&ResourcesStepDefinition, fieldWorkerMemory, "24576"},
		{&ResourcesStepDefinition, fieldWorkerRootDisk, "200"},
		{&ResourcesStepDefinition, fieldWorkerDataDisk, "750"},
		{&FilesStepDefinition, config.FieldFilesPullSecret, "/tmp/pull-secret.json"},
		{&FilesStepDefinition, config.FieldFilesSSHPublicKey, "/tmp/id_ed25519.pub"},
		{&AdvancedStepDefinition, config.FieldNetworkingPodCIDR, "10.132.0.0/14"},
		{&AdvancedStepDefinition, config.FieldNetworkingServiceCIDR, "172.31.0.0/16"},
		{&AdvancedStepDefinition, config.FieldNetworkingHostPrefix, "24"},
		{&AdvancedStepDefinition, fieldHookCommand, "/usr/local/bin/pause"},
		{&AdvancedStepDefinition, fieldHookTimeout, "20m"},
		{&AdvancedStepDefinition, fieldDrainTimeout, "1h30m"},
		{&AdvancedStepDefinition, fieldHandoff, valNo},
		{&AdvancedStepDefinition, fieldCatalogs, valNo},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			cfg := config.DefaultConfig()
			f := findField(t, tc.def, tc.key)
			if err := f.ConfigSet(cfg, tc.value); err != nil {
				t.Fatalf("ConfigSet(%q): %v", tc.value, err)
			}
			if got := f.ConfigGet(cfg); got != tc.value {
				t.Errorf("ConfigGet = %q, want %q", got, tc.value)
			}
		})
	}
}

func TestResourcesStepDefinition_DataDiskSizing(t *testing.T) {
	cfg := config.DefaultConfig()
	data := findField(t, &ResourcesStepDefinition, fieldWorkerDataDisk)

	if err := data.ConfigSet(cfg, "300"); err != nil {
		t.Fatal(err)
	}
	if got := cfg.Workers.Disks; len(got) != 2 || got[1].SizeGiB != 300 || got[1].Storage != "local-lvm" || got[1].Wipe == nil {
		t.Fatalf("resizing kept neither size nor the disk's other fields: %+v", got)
	}

	if err := data.ConfigSet(cfg, "0"); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Workers.Disks) != 1 {
		t.Fatalf("zero must drop the data disk, got %+v", cfg.Workers.Disks)
	}
	if got := data.ConfigGet(cfg); got != "0" {
		t.Errorf("ConfigGet without a data disk = %q, want 0", got)
	}

	if err := data.ConfigSet(cfg, "250"); err != nil {
		t.Fatal(err)
	}
	if got := cfg.Workers.Disks; len(got) != 2 || got[1].SizeGiB != 250 {
		t.Fatalf("a size on a one-disk list must append the data disk, got %+v", got)
	}
}

func TestProxmoxStepDefinition_SecretsStayOutOfTheConfig(t *testing.T) {
	step := NewProxmoxStep()
	step.SetValue(fieldAPIToken, "root@pam!okdctl=s3cret")
	step.SetValue(fieldInsecure, valYes)
	cfg := config.DefaultConfig()

	if err := step.Apply(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Provider.Type != config.ProviderProxmox {
		t.Errorf("Provider.Type = %q, want proxmox", cfg.Provider.Type)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "s3cret") || strings.Contains(string(data), "insecure") {
		t.Fatalf("the token or tls toggle reached the yaml:\n%s", data)
	}
	secrets := step.Secrets()
	if string(secrets.ProxmoxToken.Bytes()) != "root@pam!okdctl=s3cret" || !secrets.ProxmoxInsecure {
		t.Errorf("Secrets() = %+v, want the typed token and insecure=true", secrets)
	}
}

func TestNetworkingCrossFields(t *testing.T) {
	valid := map[string]string{
		config.FieldNetworkingMachineCIDR: "192.168.1.0/24",
		config.FieldNetworkingGateway:     "192.168.1.1",
		config.FieldNetworkingAPIVIP:      "192.168.1.158",
		config.FieldNetworkingIngressVIP:  "192.168.1.159",
		config.FieldNetworkingNodeIPStart: "192.168.1.150",
	}
	if err := networkingCrossFields(valid); err != nil {
		t.Fatalf("valid layout rejected: %v", err)
	}

	outside := map[string]string{}
	for k, v := range valid {
		outside[k] = v
	}
	outside[config.FieldNetworkingAPIVIP] = "10.0.0.5"
	if err := networkingCrossFields(outside); err == nil || !strings.Contains(err.Error(), "api vip 10.0.0.5 is outside machine cidr") {
		t.Errorf("outside-cidr vip: err = %v", err)
	}

	dup := map[string]string{}
	for k, v := range valid {
		dup[k] = v
	}
	dup[config.FieldNetworkingIngressVIP] = valid[config.FieldNetworkingAPIVIP]
	if err := networkingCrossFields(dup); err == nil || !strings.Contains(err.Error(), "api vip and ingress vip both use") {
		t.Errorf("duplicate vip: err = %v", err)
	}

	malformed := map[string]string{config.FieldNetworkingMachineCIDR: "999.999.1.0/99", config.FieldNetworkingAPIVIP: "10.0.0.5"}
	if err := networkingCrossFields(malformed); err != nil {
		t.Errorf("a malformed cidr is the field validator's to report, got %v", err)
	}
}

func TestAdvancedStep_CIDROverlapFocusesAndMarksImplicatedFields(t *testing.T) {
	step := wizard.NewDataDrivenStep(&AdvancedStepDefinition)
	step.SetFocused(true)
	_ = step.Init()
	step.SetValue(config.FieldNetworkingPodCIDR, "10.128.0.0/14")
	step.SetValue(config.FieldNetworkingServiceCIDR, "10.128.0.0/16")

	_, cmd := step.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Update(enter) with overlapping CIDRs: want a cmd, got nil")
	}
	_ = cmd()

	view := tuitest.StripANSI(step.View(120, 40))
	lines := strings.Split(view, "\n")
	span, ok := step.FocusedSpan()
	if !ok || span.Start >= len(lines) || !strings.Contains(lines[span.Start], "pod cidr") {
		t.Fatalf("focused span after the overlap error = %+v (ok=%v), want pod cidr's:\n%s", span, ok, view)
	}
	if n := strings.Count(view, "must not overlap"); n != 2 {
		t.Fatalf("View() shows the overlap error %d times, want 2 (once at each implicated field):\n%s", n, view)
	}
}
