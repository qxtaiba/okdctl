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

func TestBasicsStepDefinition_Fields(t *testing.T) {
	cfg := &config.Config{}

	name := findField(t, &BasicsStepDefinition, "cluster_name")
	_ = name.ConfigSet(cfg, "homelab")
	if cfg.Cluster.Name != "homelab" {
		t.Errorf("Cluster.Name = %q, want homelab", cfg.Cluster.Name)
	}

	domain := findField(t, &BasicsStepDefinition, fieldDomain)
	_ = domain.ConfigSet(cfg, "example.com")
	if cfg.Cluster.Domain != "example.com" {
		t.Errorf("Cluster.Domain = %q, want example.com", cfg.Cluster.Domain)
	}

	cp := findField(t, &BasicsStepDefinition, "control_plane_count")
	_ = cp.ConfigSet(cfg, "5")
	if cfg.Topology.ControlPlane.Count != 5 {
		t.Errorf("ControlPlane.Count = %d, want 5", cfg.Topology.ControlPlane.Count)
	}

	workers := findField(t, &BasicsStepDefinition, "worker_count")
	_ = workers.ConfigSet(cfg, "4")
	if cfg.Topology.Workers.Count != 4 {
		t.Errorf("Workers.Count = %d, want 4", cfg.Topology.Workers.Count)
	}
}

func TestAdvancedStepDefinition_Fields(t *testing.T) {
	cfg := &config.Config{}

	vmid := findField(t, &AdvancedStepDefinition, "vm_id_base")
	_ = vmid.ConfigSet(cfg, "7000")
	if cfg.Topology.VMIDBase != 7000 {
		t.Errorf("VMIDBase = %d, want 7000", cfg.Topology.VMIDBase)
	}

	tfEnv := findField(t, &AdvancedStepDefinition, "terraform_env")
	if err := tfEnv.ConfigSet(cfg, "staging"); err != nil {
		t.Fatalf("ConfigSet(terraform_env): %v", err)
	}
	if cfg.Deployment.TerraformEnv != "staging" {
		t.Errorf("TerraformEnv = %q, want staging", cfg.Deployment.TerraformEnv)
	}
	if got := tfEnv.ConfigGet(cfg); got != "staging" {
		t.Errorf("ConfigGet(terraform_env) = %q", got)
	}
	if tfEnv.Label == "terraform workspace" {
		t.Error("terraform_env field is still labelled a terraform workspace")
	}

	binDir := findField(t, &AdvancedStepDefinition, "bin_dir")
	_ = binDir.ConfigSet(cfg, "/opt/okdctl/bin")
	if cfg.Deployment.BinDir != "/opt/okdctl/bin" {
		t.Errorf("BinDir = %q, want /opt/okdctl/bin", cfg.Deployment.BinDir)
	}

	approve := findField(t, &AdvancedStepDefinition, "auto_approve")
	_ = approve.ConfigSet(cfg, "yes")
	if !cfg.Deployment.AutoApprove {
		t.Error("AutoApprove = false, want true")
	}

	ntp := findField(t, &AdvancedStepDefinition, "ntp_server")
	if err := ntp.ConfigSet(cfg, "192.168.1.20"); err != nil {
		t.Fatalf("ConfigSet(ntp_server): %v", err)
	}
	if cfg.Networking.NTPServer != "192.168.1.20" {
		t.Errorf("NTPServer = %q, want 192.168.1.20", cfg.Networking.NTPServer)
	}
	if got := ntp.ConfigGet(cfg); got != "192.168.1.20" {
		t.Errorf("ConfigGet(ntp_server) = %q", got)
	}
	if err := ntp.Validate("!not valid!"); err == nil {
		t.Error("Validate(ntp_server) accepted invalid host")
	}

	cfg.Provider.Proxmox = &config.ProxmoxConfig{}
	ha := findField(t, &AdvancedStepDefinition, "ha_enabled")
	if err := ha.ConfigSet(cfg, "yes"); err != nil {
		t.Fatalf("ConfigSet(ha_enabled): %v", err)
	}
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

func TestAdvancedStepDefinition_NoDeadDebugFields(t *testing.T) {
	for _, section := range AdvancedStepDefinition.Sections {
		for _, field := range section.Fields {
			if field.Key == "debug" || field.Key == "skip_deps_check" {
				t.Errorf("dead field %q still declared in AdvancedStepDefinition", field.Key)
			}
		}
	}
}

func TestFilesStepDefinition_Fields(t *testing.T) {
	cfg := &config.Config{}

	pullSecret := findField(t, &FilesStepDefinition, "pull_secret")
	if err := pullSecret.ConfigSet(cfg, "/etc/okd/pull-secret.json"); err != nil {
		t.Fatalf("ConfigSet(pull_secret): %v", err)
	}
	if cfg.Files.PullSecret != "/etc/okd/pull-secret.json" {
		t.Errorf("Files.PullSecret = %q", cfg.Files.PullSecret)
	}

	sshKey := findField(t, &FilesStepDefinition, "ssh_public_key")
	_ = sshKey.ConfigSet(cfg, "/etc/okd/id_ed25519.pub")
	if got := sshKey.ConfigGet(cfg); got != "/etc/okd/id_ed25519.pub" {
		t.Errorf("ConfigGet(ssh_public_key) = %q", got)
	}

	root := findField(t, &FilesStepDefinition, "web_root")
	_ = root.ConfigSet(cfg, "/srv/ignition")
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

func TestNetworkingStepDefinition_Fields(t *testing.T) {
	cfg := &config.Config{}

	mc := findField(t, &NetworkingStepDefinition, "machine_cidr")
	_ = mc.ConfigSet(cfg, "192.168.1.0/24")
	if cfg.Networking.MachineCIDR != "192.168.1.0/24" {
		t.Errorf("MachineCIDR = %q", cfg.Networking.MachineCIDR)
	}

	gw := findField(t, &NetworkingStepDefinition, fieldGateway)
	_ = gw.ConfigSet(cfg, "192.168.1.1")
	if cfg.Networking.Gateway != "192.168.1.1" {
		t.Errorf("Gateway = %q", cfg.Networking.Gateway)
	}

	dns := findField(t, &NetworkingStepDefinition, "dns_servers")
	if err := dns.ConfigSet(cfg, "192.168.1.1, 8.8.8.8,"); err != nil {
		t.Fatalf("ConfigSet(dns_servers): %v", err)
	}
	if got := cfg.Networking.DNS; len(got) != 2 || got[0] != "192.168.1.1" || got[1] != "8.8.8.8" {
		t.Errorf("Networking.DNS = %v, want [192.168.1.1 8.8.8.8]", got)
	}
	if got := dns.ConfigGet(cfg); got != "192.168.1.1, 8.8.8.8" {
		t.Errorf("ConfigGet(dns_servers) = %q", got)
	}

	bastion := findField(t, &NetworkingStepDefinition, "bastion_ip")
	_ = bastion.ConfigSet(cfg, DefaultBastionIP)
	if cfg.Networking.Bastion.IP != DefaultBastionIP {
		t.Errorf("Bastion.IP = %q", cfg.Networking.Bastion.IP)
	}

	start := findField(t, &NetworkingStepDefinition, "start_ip")
	_ = start.ConfigSet(cfg, "192.168.1.140")
	if cfg.Networking.StaticIP.Start != "192.168.1.140" {
		t.Errorf("StaticIP.Start = %q", cfg.Networking.StaticIP.Start)
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

	host := findField(t, &ProxmoxStepDefinition, fieldHost)
	if err := host.ConfigSet(cfg, "10.0.0.5:8006"); err != nil {
		t.Fatalf("ConfigSet(host): %v", err)
	}
	if cfg.Provider.Proxmox == nil || cfg.Provider.Proxmox.Host != "10.0.0.5:8006" {
		t.Fatalf("Provider.Proxmox.Host not set: %+v", cfg.Provider.Proxmox)
	}
	if got := host.ConfigGet(cfg); got != "10.0.0.5:8006" {
		t.Errorf("ConfigGet(host) = %q", got)
	}

	user := findField(t, &ProxmoxStepDefinition, "username")
	_ = user.ConfigSet(cfg, "root@pam")
	if cfg.Provider.Proxmox.Username != "root@pam" {
		t.Errorf("Username = %q", cfg.Provider.Proxmox.Username)
	}

	pass := findField(t, &ProxmoxStepDefinition, "password")
	if err := pass.ConfigSet(cfg, "s3cret"); err != nil {
		t.Fatalf("ConfigSet(password): %v", err)
	}
	if cfg.Provider.Proxmox.Password.IsEmpty() {
		t.Error("Password not set")
	}

	insecure := findField(t, &ProxmoxStepDefinition, "skip_tls_verify")
	_ = insecure.ConfigSet(cfg, "yes")
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

func TestResourcesStepDefinition_Fields(t *testing.T) {
	cfg := &config.Config{}

	cpu := findField(t, &ResourcesStepDefinition, "cp_vcpus")
	_ = cpu.ConfigSet(cfg, "6")
	if cfg.Topology.ControlPlane.CPU != 6 {
		t.Errorf("ControlPlane.CPU = %d, want 6", cfg.Topology.ControlPlane.CPU)
	}

	disk := findField(t, &ResourcesStepDefinition, "cp_disk")
	if err := disk.ConfigSet(cfg, "77"); err != nil {
		t.Fatalf("ConfigSet(cp_disk): %v", err)
	}
	if cfg.Topology.ControlPlane.DiskGB != 77 {
		t.Errorf("ControlPlane.Disk = %d, want 77", cfg.Topology.ControlPlane.DiskGB)
	}
	if cfg.Topology.Bootstrap.DiskGB != 77 {
		t.Errorf("Bootstrap.Disk = %d, want 77 (cp_disk must also seed bootstrap disk)", cfg.Topology.Bootstrap.DiskGB)
	}

	workerDataDisk := findField(t, &ResourcesStepDefinition, "worker_data_disk")
	_ = workerDataDisk.ConfigSet(cfg, "1000")
	if cfg.Disks.WorkerDataSizeGB != 1000 {
		t.Errorf("Disks.WorkerDataSizeGB = %d, want 1000", cfg.Disks.WorkerDataSizeGB)
	}

	cpDataDisk := findField(t, &ResourcesStepDefinition, "cp_data_disk")
	_ = cpDataDisk.ConfigSet(cfg, "200")
	if cfg.Disks.ControlPlaneDataSizeGB != 200 {
		t.Errorf("Disks.ControlPlaneDataSizeGB = %d, want 200", cfg.Disks.ControlPlaneDataSizeGB)
	}
}

func TestAddonHelpers_LazyInitNilMap(t *testing.T) {
	cfg := &config.Config{} // Addons is nil

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

	// Getters on an addon/key that was never set must not panic.
	if got := addonEnabled("nonexistent")(cfg); got != valNo {
		t.Fatalf("addonEnabled(nonexistent) = %q, want no", got)
	}
	if got := addonSetting("flux", "missing-key")(cfg); got != "" {
		t.Fatalf("addonSetting(missing key) = %q, want empty", got)
	}
}

func TestAddonsStepDefinition_FieldWiring(t *testing.T) {
	cfg := &config.Config{}

	vaults := findField(t, &AddonsStepDefinition, "secretstore_op_vaults")
	if err := vaults.ConfigSet(cfg, "homelab=1,shared=2"); err != nil {
		t.Fatalf("ConfigSet(secretstore_op_vaults): %v", err)
	}
	if got := vaults.ConfigGet(cfg); got != "homelab=1,shared=2" {
		t.Fatalf("ConfigGet(secretstore_op_vaults) = %q, want homelab=1,shared=2", got)
	}

	fluxEnabled := findField(t, &AddonsStepDefinition, "flux_enabled")
	if err := fluxEnabled.ConfigSet(cfg, "yes"); err != nil {
		t.Fatalf("ConfigSet(flux_enabled): %v", err)
	}
	if !cfg.Addons["flux"].Enabled {
		t.Fatal("flux.Enabled = false after ConfigSet(yes)")
	}
}
