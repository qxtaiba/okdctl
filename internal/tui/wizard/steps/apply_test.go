package steps

import (
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
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

func setField(t *testing.T, def *wizard.StepDefinition, cfg *config.Config, key, value string) wizard.FieldDefinition {
	t.Helper()
	f := findField(t, def, key)
	if err := f.ConfigSet(cfg, value); err != nil {
		t.Fatalf("ConfigSet(%s, %q): %v", key, value, err)
	}
	return f
}

func TestBasicsStepDefinition_Fields(t *testing.T) {
	cfg := &config.Config{}

	setField(t, &BasicsStepDefinition, cfg, "control_plane_count", "5")
	if cfg.Topology.ControlPlane.Count != 5 {
		t.Errorf("ControlPlane.Count = %d, want 5", cfg.Topology.ControlPlane.Count)
	}
}

func TestAdvancedStepDefinition_Fields(t *testing.T) {
	cfg := &config.Config{}

	setField(t, &AdvancedStepDefinition, cfg, "auto_approve", "yes")
	if !cfg.Deployment.AutoApprove {
		t.Error("AutoApprove = false, want true")
	}

	cfg.Provider.Proxmox = &config.ProxmoxConfig{}
	ha := setField(t, &AdvancedStepDefinition, cfg, "ha_enabled", "yes")
	if !cfg.Provider.Proxmox.HAEnabled {
		t.Error("Provider.Proxmox.HAEnabled = false, want true")
	}
	if got := ha.ConfigGet(cfg); got != valYes {
		t.Errorf("ConfigGet(ha_enabled) = %q, want yes", got)
	}
}

func TestAdvancedStepDefinition_HAEnabledOnNilProxmoxConfig(t *testing.T) {
	cfg := &config.Config{}
	ha := findField(t, &AdvancedStepDefinition, "ha_enabled")
	if got := ha.ConfigGet(cfg); got != valNo {
		t.Errorf("ConfigGet(ha_enabled) on nil Proxmox = %q, want no", got)
	}
	if err := ha.ConfigSet(cfg, "yes"); err != nil {
		t.Fatalf("ConfigSet(ha_enabled): %v", err)
	}
	if cfg.Provider.Proxmox != nil {
		t.Error("ConfigSet(ha_enabled) on nil Proxmox must not allocate a ProxmoxConfig")
	}
}

func TestFilesStepDefinition_Fields(t *testing.T) {
	cfg := &config.Config{}

	setField(t, &FilesStepDefinition, cfg, "web_root", "/srv/ignition")
	if cfg.HTTPServer.Root != "/srv/ignition" {
		t.Errorf("HTTPServer.Root = %q", cfg.HTTPServer.Root)
	}
}

func TestFilesStepDefinition_ApplySyncsIgnitionIP(t *testing.T) {
	cfg := &config.Config{}
	cfg.Networking.Bastion.IP = "10.0.0.5"

	if err := FilesStepDefinition.Apply(nil, cfg); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if cfg.HTTPServer.IgnitionServerIP != "10.0.0.5" {
		t.Errorf("HTTPServer.IgnitionServerIP = %q, want 10.0.0.5", cfg.HTTPServer.IgnitionServerIP)
	}
}

func TestFilesStepDefinition_ShouldShow(t *testing.T) {
	cfg := &config.Config{Distribution: config.DistributionConfig{Type: config.DistributionOKD}}
	if !FilesStepDefinition.ShouldShow(cfg) {
		t.Error("ShouldShow = false for OKD distribution, want true")
	}
	cfg.Distribution.Type = "other"
	if FilesStepDefinition.ShouldShow(cfg) {
		t.Error("ShouldShow = true for non-OKD distribution, want false")
	}
}

func TestNetworkingStepDefinition_DNSServersRoundTrip(t *testing.T) {
	cfg := &config.Config{}

	dns := setField(t, &NetworkingStepDefinition, cfg, "dns_servers", "192.168.1.1, 8.8.8.8,")
	if got := cfg.Networking.DNS; len(got) != 2 || got[0] != "192.168.1.1" || got[1] != "8.8.8.8" {
		t.Errorf("Networking.DNS = %v, want [192.168.1.1 8.8.8.8]", got)
	}
	if got := dns.ConfigGet(cfg); got != "192.168.1.1, 8.8.8.8" {
		t.Errorf("ConfigGet(dns_servers) = %q", got)
	}
}

func TestNetworkingStepDefinition_Validate(t *testing.T) {
	valid := map[string]string{
		"machine_cidr": "192.168.1.0/24",
		"pod_cidr":     "10.128.0.0/14",
		"service_cidr": "172.30.0.0/16",
		fieldGateway:   "192.168.1.1",
	}
	if err := NetworkingStepDefinition.Validate(valid); err != nil {
		t.Fatalf("Validate(valid) = %v, want nil", err)
	}

	overlap := map[string]string{
		"machine_cidr": "192.168.1.0/24",
		"pod_cidr":     "192.168.1.0/24",
		"service_cidr": "172.30.0.0/16",
		fieldGateway:   "192.168.1.1",
	}
	if err := NetworkingStepDefinition.Validate(overlap); err == nil {
		t.Fatal("Validate(overlapping CIDRs) = nil, want error")
	}

	badGateway := map[string]string{
		"machine_cidr": "192.168.1.0/24",
		"pod_cidr":     "10.128.0.0/14",
		"service_cidr": "172.30.0.0/16",
		fieldGateway:   "10.0.0.1",
	}
	if err := NetworkingStepDefinition.Validate(badGateway); err == nil {
		t.Fatal("Validate(gateway outside machine CIDR) = nil, want error")
	}
}

func TestNetworkingStepDefinition_ApplyDerivesStaticIPFields(t *testing.T) {
	cfg := &config.Config{}
	cfg.Networking.MachineCIDR = "192.168.1.0/24"
	cfg.Networking.Bastion.IP = DefaultBastionIP

	if err := NetworkingStepDefinition.Apply(nil, cfg); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if cfg.Networking.StaticIP.DNS != DefaultBastionIP {
		t.Errorf("StaticIP.DNS = %q, want 192.168.1.20", cfg.Networking.StaticIP.DNS)
	}
	if cfg.Networking.StaticIP.Netmask != "255.255.255.0" {
		t.Errorf("StaticIP.Netmask = %q, want 255.255.255.0", cfg.Networking.StaticIP.Netmask)
	}
}

func TestNetworkingStepDefinition_ApplyRejectsBadCIDR(t *testing.T) {
	cfg := &config.Config{}
	cfg.Networking.MachineCIDR = "not-a-cidr"
	if err := NetworkingStepDefinition.Apply(nil, cfg); err == nil {
		t.Fatal("Apply with invalid machine CIDR = nil error, want error")
	}
}

func TestProxmoxStepDefinition_Fields(t *testing.T) {
	cfg := &config.Config{}

	host := setField(t, &ProxmoxStepDefinition, cfg, fieldHost, "10.0.0.5:8006")
	if cfg.Provider.Proxmox == nil || cfg.Provider.Proxmox.Host != "10.0.0.5:8006" {
		t.Fatalf("Provider.Proxmox.Host not set: %+v", cfg.Provider.Proxmox)
	}
	if got := host.ConfigGet(cfg); got != "10.0.0.5:8006" {
		t.Errorf("ConfigGet(host) = %q", got)
	}

	setField(t, &ProxmoxStepDefinition, cfg, "username", "root@pam")
	if cfg.Provider.Proxmox.Username != "root@pam" {
		t.Errorf("Username = %q", cfg.Provider.Proxmox.Username)
	}

	setField(t, &ProxmoxStepDefinition, cfg, "password", "s3cret")
	if cfg.Provider.Proxmox.Password.IsEmpty() {
		t.Error("Password not set")
	}

	insecure := setField(t, &ProxmoxStepDefinition, cfg, "skip_tls_verify", "yes")
	if !cfg.Provider.Proxmox.Insecure {
		t.Error("Insecure = false, want true")
	}
	if got := insecure.ConfigGet(cfg); got != valYes {
		t.Errorf("ConfigGet(skip_tls_verify) = %q, want yes", got)
	}
}

func TestProxmoxStepDefinition_FieldsOnNilProxmoxConfig(t *testing.T) {
	cfg := &config.Config{}
	insecure := findField(t, &ProxmoxStepDefinition, "skip_tls_verify")
	if got := insecure.ConfigGet(cfg); got != valNo {
		t.Errorf("ConfigGet(skip_tls_verify) on nil Proxmox = %q, want no", got)
	}
}

func TestProxmoxStepDefinition_ApplySetsProviderType(t *testing.T) {
	cfg := &config.Config{}
	if err := ProxmoxStepDefinition.Apply(nil, cfg); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if cfg.Provider.Type != config.ProviderProxmox {
		t.Errorf("Provider.Type = %q, want %q", cfg.Provider.Type, config.ProviderProxmox)
	}
}

func TestResourcesStepDefinition_CPDiskSeedsBootstrap(t *testing.T) {
	cfg := &config.Config{}

	setField(t, &ResourcesStepDefinition, cfg, "cp_disk", "77")
	if cfg.Topology.ControlPlane.DiskGB != 77 {
		t.Errorf("ControlPlane.Disk = %d, want 77", cfg.Topology.ControlPlane.DiskGB)
	}
	if cfg.Topology.Bootstrap.DiskGB != 77 {
		t.Errorf("Bootstrap.Disk = %d, want 77 (cp_disk must also seed bootstrap disk)", cfg.Topology.Bootstrap.DiskGB)
	}
}

func TestAddonHelpers_LazyInitNilMap(t *testing.T) {
	cfg := &config.Config{}

	if err := setAddonEnabled("flux")(cfg, "yes"); err != nil {
		t.Fatalf("setAddonEnabled: %v", err)
	}
	if !cfg.Addons["flux"].Enabled {
		t.Fatal("flux.Enabled = false, want true")
	}

	if err := setAddonSetting("flux", "branch")(cfg, "main"); err != nil {
		t.Fatalf("setAddonSetting: %v", err)
	}
	if got := cfg.Addons["flux"].Settings["branch"]; got != "main" {
		t.Fatalf("Settings[branch] = %q, want main", got)
	}

	if got := addonEnabled("flux")(cfg); got != valYes {
		t.Fatalf("addonEnabled = %q, want yes", got)
	}
	if got := addonSetting("flux", "branch")(cfg); got != "main" {
		t.Fatalf("addonSetting = %q, want main", got)
	}

	if got := addonEnabled("nonexistent")(cfg); got != valNo {
		t.Fatalf("addonEnabled(nonexistent) = %q, want no", got)
	}
	if got := addonSetting("flux", "missing-key")(cfg); got != "" {
		t.Fatalf("addonSetting(missing key) = %q, want empty", got)
	}
}

func TestAddonsStepDefinition_FieldWiring(t *testing.T) {
	cfg := &config.Config{}

	vaults := setField(t, &AddonsStepDefinition, cfg, "secretstore_op_vaults", "homelab=1,shared=2")
	if got := vaults.ConfigGet(cfg); got != "homelab=1,shared=2" {
		t.Fatalf("ConfigGet(secretstore_op_vaults) = %q, want homelab=1,shared=2", got)
	}

	setField(t, &AddonsStepDefinition, cfg, "flux_enabled", "yes")
	if !cfg.Addons["flux"].Enabled {
		t.Fatal("flux.Enabled = false after ConfigSet(yes)")
	}
}
