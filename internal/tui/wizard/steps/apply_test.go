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

	name := setField(t, &BasicsStepDefinition, cfg, "cluster_name", "homelab")
	if cfg.Cluster.Name != "homelab" {
		t.Errorf("Cluster.Name = %q, want homelab", cfg.Cluster.Name)
	}
	if got := name.ConfigGet(cfg); got != "homelab" {
		t.Errorf("ConfigGet(cluster_name) = %q, want homelab", got)
	}

	domain := setField(t, &BasicsStepDefinition, cfg, fieldDomain, "prod.example")
	if cfg.Cluster.Domain != "prod.example" {
		t.Errorf("Cluster.Domain = %q, want prod.example", cfg.Cluster.Domain)
	}
	if got := domain.ConfigGet(cfg); got != "prod.example" {
		t.Errorf("ConfigGet(domain) = %q, want prod.example", got)
	}

	workers := setField(t, &BasicsStepDefinition, cfg, "worker_count", "5")
	if cfg.Topology.Workers.Count != 5 {
		t.Errorf("Workers.Count = %d, want 5", cfg.Topology.Workers.Count)
	}
	if got := workers.ConfigGet(cfg); got != "5" {
		t.Errorf("ConfigGet(worker_count) = %q, want 5", got)
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

	vmid := setField(t, &AdvancedStepDefinition, cfg, "vm_id_base", "7000")
	if cfg.Topology.VMIDBase != 7000 {
		t.Errorf("Topology.VMIDBase = %d, want 7000", cfg.Topology.VMIDBase)
	}
	if got := vmid.ConfigGet(cfg); got != "7000" {
		t.Errorf("ConfigGet(vm_id_base) = %q, want 7000", got)
	}

	cpuType := setField(t, &AdvancedStepDefinition, cfg, "cpu_type", "x86-64-v2")
	if cfg.Provider.Proxmox.CPUType != "x86-64-v2" {
		t.Errorf("Provider.Proxmox.CPUType = %q, want x86-64-v2", cfg.Provider.Proxmox.CPUType)
	}
	if got := cpuType.ConfigGet(cfg); got != "x86-64-v2" {
		t.Errorf("ConfigGet(cpu_type) = %q, want x86-64-v2", got)
	}

	numa := setField(t, &AdvancedStepDefinition, cfg, "numa_enabled", "yes")
	if !cfg.Provider.Proxmox.NUMAEnabled {
		t.Error("Provider.Proxmox.NUMAEnabled = false, want true")
	}
	if got := numa.ConfigGet(cfg); got != valYes {
		t.Errorf("ConfigGet(numa_enabled) = %q, want yes", got)
	}

	ntp := setField(t, &AdvancedStepDefinition, cfg, "ntp_server", "time.example.com")
	if cfg.Networking.NTPServer != "time.example.com" {
		t.Errorf("Networking.NTPServer = %q, want time.example.com", cfg.Networking.NTPServer)
	}
	if got := ntp.ConfigGet(cfg); got != "time.example.com" {
		t.Errorf("ConfigGet(ntp_server) = %q, want time.example.com", got)
	}

	bootstrapTimeout := setField(t, &AdvancedStepDefinition, cfg, "bootstrap_timeout", "1800")
	if cfg.Deployment.BootstrapTimeout != 1800 {
		t.Errorf("Deployment.BootstrapTimeout = %d, want 1800", cfg.Deployment.BootstrapTimeout)
	}
	if got := bootstrapTimeout.ConfigGet(cfg); got != "1800" {
		t.Errorf("ConfigGet(bootstrap_timeout) = %q, want 1800", got)
	}

	installTimeout := setField(t, &AdvancedStepDefinition, cfg, "install_timeout", "3600")
	if cfg.Deployment.InstallTimeout != 3600 {
		t.Errorf("Deployment.InstallTimeout = %d, want 3600", cfg.Deployment.InstallTimeout)
	}
	if got := installTimeout.ConfigGet(cfg); got != "3600" {
		t.Errorf("ConfigGet(install_timeout) = %q, want 3600", got)
	}

	terraformEnv := setField(t, &AdvancedStepDefinition, cfg, "terraform_env", "staging")
	if cfg.Deployment.TerraformEnv != "staging" {
		t.Errorf("Deployment.TerraformEnv = %q, want staging", cfg.Deployment.TerraformEnv)
	}
	if got := terraformEnv.ConfigGet(cfg); got != "staging" {
		t.Errorf("ConfigGet(terraform_env) = %q, want staging", got)
	}

	binDir := setField(t, &AdvancedStepDefinition, cfg, "bin_dir", "/opt/bin")
	if cfg.Deployment.BinDir != "/opt/bin" {
		t.Errorf("Deployment.BinDir = %q, want /opt/bin", cfg.Deployment.BinDir)
	}
	if got := binDir.ConfigGet(cfg); got != "/opt/bin" {
		t.Errorf("ConfigGet(bin_dir) = %q, want /opt/bin", got)
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

	pullSecret := setField(t, &FilesStepDefinition, cfg, "pull_secret", "/tmp/pull-secret.json")
	if cfg.Files.PullSecret != "/tmp/pull-secret.json" {
		t.Errorf("Files.PullSecret = %q, want /tmp/pull-secret.json", cfg.Files.PullSecret)
	}
	if got := pullSecret.ConfigGet(cfg); got != "/tmp/pull-secret.json" {
		t.Errorf("ConfigGet(pull_secret) = %q, want /tmp/pull-secret.json", got)
	}

	sshKey := setField(t, &FilesStepDefinition, cfg, "ssh_public_key", "/tmp/id_ed25519.pub")
	if cfg.Files.SSHPublicKey != "/tmp/id_ed25519.pub" {
		t.Errorf("Files.SSHPublicKey = %q, want /tmp/id_ed25519.pub", cfg.Files.SSHPublicKey)
	}
	if got := sshKey.ConfigGet(cfg); got != "/tmp/id_ed25519.pub" {
		t.Errorf("ConfigGet(ssh_public_key) = %q, want /tmp/id_ed25519.pub", got)
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

// TestNetworkingStepDefinition_FieldsRoundTrip extends dns_servers' own
// round-trip test above to every other networking field: each gets a fresh
// Config, a non-default ConfigSet value, and a ConfigGet assertion that
// value comes back unchanged.
func TestNetworkingStepDefinition_FieldsRoundTrip(t *testing.T) {
	cases := []struct{ key, value string }{
		{"machine_cidr", "10.0.0.0/24"},
		{fieldGateway, "10.0.0.1"},
		{"pod_cidr", "10.132.0.0/14"},
		{"service_cidr", "172.31.0.0/16"},
		{"host_prefix", "24"},
		{"start_ip", "10.0.0.50"},
		{fieldInterface, "eth0"},
		{"bastion_ip", "10.0.0.20"},
		{"vip", "10.0.0.99"},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			cfg := &config.Config{}
			f := setField(t, &NetworkingStepDefinition, cfg, tc.key, tc.value)
			if got := f.ConfigGet(cfg); got != tc.value {
				t.Errorf("ConfigGet(%s) = %q, want %q", tc.key, got, tc.value)
			}
		})
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
	err := NetworkingStepDefinition.Validate(overlap)
	// R4: the overlap message names its fix so the operator isn't left
	// guessing what to do about it.
	const wantOverlapErr = "machine cidr and pod cidr must not overlap — widen or move one of the ranges"
	if err == nil || err.Error() != wantOverlapErr {
		t.Fatalf("Validate(overlapping CIDRs) = %v, want %q", err, wantOverlapErr)
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

	tokenID := setField(t, &ProxmoxStepDefinition, cfg, "token_id", "user@pve!okdctl")
	if cfg.Provider.Proxmox.TokenID != "user@pve!okdctl" {
		t.Errorf("TokenID = %q, want user@pve!okdctl", cfg.Provider.Proxmox.TokenID)
	}
	if got := tokenID.ConfigGet(cfg); got != "user@pve!okdctl" {
		t.Errorf("ConfigGet(token_id) = %q, want user@pve!okdctl", got)
	}

	// password is intentionally write-only (ConfigGet is nil): it must never
	// be read back into the form, so there is no round-trip to assert here.
}

func TestProxmoxStepDefinition_FieldsOnNilProxmoxConfig(t *testing.T) {
	cfg := &config.Config{}
	insecure := findField(t, &ProxmoxStepDefinition, "skip_tls_verify")
	if got := insecure.ConfigGet(cfg); got != valNo {
		t.Errorf("ConfigGet(skip_tls_verify) on nil Proxmox = %q, want no", got)
	}
}

// TestProxmoxStepDefinition_AnsweredExcludesCredentials pins the context
// pane's credential-safety contract: host and username echo, password and
// token_id never do, no matter what values are present.
func TestProxmoxStepDefinition_AnsweredExcludesCredentials(t *testing.T) {
	values := map[string]string{
		fieldHost:  "10.0.0.5:8006",
		"username": "root@pam",
		"password": "s3cret",
		"token_id": "root@pam!okdctl",
	}

	facts := ProxmoxStepDefinition.Answered(values)

	want := map[string]string{"host": "10.0.0.5:8006", "username": "root@pam"}
	if len(facts) != len(want) {
		t.Fatalf("Answered() = %+v, want exactly %+v", facts, want)
	}
	for _, f := range facts {
		if want[f.Key] != f.Value {
			t.Errorf("fact %q = %q, want %q", f.Key, f.Value, want[f.Key])
		}
	}

	for _, f := range facts {
		if f.Key == "password" || f.Key == "token_id" || f.Value == values["password"] || f.Value == values["token_id"] {
			t.Fatalf("Answered() leaked a credential: %+v", facts)
		}
	}
}

func TestProxmoxStepDefinition_AnsweredOmitsBlankFields(t *testing.T) {
	if facts := ProxmoxStepDefinition.Answered(map[string]string{}); len(facts) != 0 {
		t.Fatalf("Answered({}) = %+v, want none", facts)
	}
}

func TestBasicsStepDefinition_Answered(t *testing.T) {
	values := map[string]string{
		"cluster_name":        "homelab",
		fieldDomain:           "k8s.local",
		"control_plane_count": "3",
		"worker_count":        "3",
	}

	facts := BasicsStepDefinition.Answered(values)
	want := map[string]string{
		"cluster":       "homelab",
		"domain":        "k8s.local",
		"control plane": "3",
		"workers":       "3",
	}
	if len(facts) != len(want) {
		t.Fatalf("Answered() = %+v, want exactly %+v", facts, want)
	}
	for _, f := range facts {
		if want[f.Key] != f.Value {
			t.Errorf("fact %q = %q, want %q", f.Key, f.Value, want[f.Key])
		}
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

	cpVCPUs := setField(t, &ResourcesStepDefinition, cfg, "cp_vcpus", "6")
	if cfg.Topology.ControlPlane.CPU != 6 {
		t.Errorf("ControlPlane.CPU = %d, want 6", cfg.Topology.ControlPlane.CPU)
	}
	if got := cpVCPUs.ConfigGet(cfg); got != "6" {
		t.Errorf("ConfigGet(cp_vcpus) = %q, want 6", got)
	}

	cpMemory := setField(t, &ResourcesStepDefinition, cfg, "cp_memory", "16384")
	if cfg.Topology.ControlPlane.MemoryMB != 16384 {
		t.Errorf("ControlPlane.MemoryMB = %d, want 16384", cfg.Topology.ControlPlane.MemoryMB)
	}
	if got := cpMemory.ConfigGet(cfg); got != "16384" {
		t.Errorf("ConfigGet(cp_memory) = %q, want 16384", got)
	}

	cpDisk := setField(t, &ResourcesStepDefinition, cfg, "cp_disk", "80")
	if cfg.Topology.ControlPlane.DiskGB != 80 {
		t.Errorf("ControlPlane.DiskGB = %d, want 80", cfg.Topology.ControlPlane.DiskGB)
	}
	if got := cpDisk.ConfigGet(cfg); got != "80" {
		t.Errorf("ConfigGet(cp_disk) = %q, want 80", got)
	}

	workerVCPUs := setField(t, &ResourcesStepDefinition, cfg, "worker_vcpus", "12")
	if cfg.Topology.Workers.CPU != 12 {
		t.Errorf("Workers.CPU = %d, want 12", cfg.Topology.Workers.CPU)
	}
	if got := workerVCPUs.ConfigGet(cfg); got != "12" {
		t.Errorf("ConfigGet(worker_vcpus) = %q, want 12", got)
	}

	workerMemory := setField(t, &ResourcesStepDefinition, cfg, "worker_memory", "24576")
	if cfg.Topology.Workers.MemoryMB != 24576 {
		t.Errorf("Workers.MemoryMB = %d, want 24576", cfg.Topology.Workers.MemoryMB)
	}
	if got := workerMemory.ConfigGet(cfg); got != "24576" {
		t.Errorf("ConfigGet(worker_memory) = %q, want 24576", got)
	}

	workerDisk := setField(t, &ResourcesStepDefinition, cfg, "worker_disk", "100")
	if cfg.Topology.Workers.DiskGB != 100 {
		t.Errorf("Workers.DiskGB = %d, want 100", cfg.Topology.Workers.DiskGB)
	}
	if got := workerDisk.ConfigGet(cfg); got != "100" {
		t.Errorf("ConfigGet(worker_disk) = %q, want 100", got)
	}

	workerDataDisk := setField(t, &ResourcesStepDefinition, cfg, "worker_data_disk", "750")
	if cfg.Disks.WorkerDataSizeGB != 750 {
		t.Errorf("Disks.WorkerDataSizeGB = %d, want 750", cfg.Disks.WorkerDataSizeGB)
	}
	if got := workerDataDisk.ConfigGet(cfg); got != "750" {
		t.Errorf("ConfigGet(worker_data_disk) = %q, want 750", got)
	}

	cpDataDisk := setField(t, &ResourcesStepDefinition, cfg, "cp_data_disk", "25")
	if cfg.Disks.ControlPlaneDataSizeGB != 25 {
		t.Errorf("Disks.ControlPlaneDataSizeGB = %d, want 25", cfg.Disks.ControlPlaneDataSizeGB)
	}
	if got := cpDataDisk.ConfigGet(cfg); got != "25" {
		t.Errorf("ConfigGet(cp_data_disk) = %q, want 25", got)
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

// TestAddonsStepDefinition_FieldsRoundTrip covers every remaining addons
// field: each gets a fresh Config, a non-default ConfigSet value, and a
// ConfigGet assertion that value comes back unchanged.
func TestAddonsStepDefinition_FieldsRoundTrip(t *testing.T) {
	cases := []struct{ key, value string }{
		{"flux_enabled", valYes},
		{"flux_repository", "ssh://git@github.com/example/repo.git"},
		{"flux_branch", "develop"},
		{"flux_path", "clusters/prod"},
		{"secretstore_enabled", valYes},
		{"secretstore_provider", providerVault},
		{"secretstore_secrets_dir", "config/secrets"},
		{"secretstore_op_connect_host", "http://1password:8080"},
		{"secretstore_op_vaults", "homelab=1,shared=2"},
		{"secretstore_vault_server", "https://vault.example.com"},
		{"secretstore_vault_path", "kv"},
		{"secretstore_vault_version", "v1"},
		{"secretstore_bw_org_id", "org-123"},
		{"secretstore_bw_project_id", "proj-456"},
		{"secretstore_bw_api_url", "https://api.bitwarden.example.com"},
		{"secretstore_bw_identity_url", "https://identity.bitwarden.example.com"},
		{"secretstore_bw_sdk_url", "https://sdk.bitwarden.example.com"},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			cfg := &config.Config{}
			f := setField(t, &AddonsStepDefinition, cfg, tc.key, tc.value)
			if got := f.ConfigGet(cfg); got != tc.value {
				t.Errorf("ConfigGet(%s) = %q, want %q", tc.key, got, tc.value)
			}
		})
	}
}
