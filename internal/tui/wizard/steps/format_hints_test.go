package steps

import (
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// fieldByKey returns the field with key from def, failing the test if none
// is found — a typo in the table should fail loudly rather than silently
// no-op.
func fieldByKey(t *testing.T, def *wizard.StepDefinition, key string) wizard.FieldDefinition {
	t.Helper()
	for _, sec := range def.Sections {
		for i := range sec.Fields {
			if sec.Fields[i].Key == key {
				return sec.Fields[i]
			}
		}
	}
	t.Fatalf("step %q has no field %q", def.ID, key)
	return wizard.FieldDefinition{}
}

// TestFormatShapedFields_HelpStatesShape asserts that every field whose
// value has a required shape (a validator enforcing a specific format, not
// just a free-text label) carries a Help row that states the shape — since
// the placeholder that otherwise hints at it disappears once the user
// starts typing.
func TestFormatShapedFields_HelpStatesShape(t *testing.T) {
	cases := []struct {
		step  *wizard.StepDefinition
		key   string
		token string
	}{
		{&BasicsStepDefinition, "domain", "example.com"},
		{&BasicsStepDefinition, "worker_count", "0-100"},
		{&ProxmoxStepDefinition, fieldHost, "ip:port"},
		{&ProxmoxStepDefinition, "username", "user@realm"},
		{&ProxmoxStepDefinition, "token_id", "user@realm!tokenname"},
		{&ProxmoxStepDefinition, "token_id", "PROXMOX_VE_API_TOKEN in id=secret form"},
		{&NetworkingStepDefinition, "machine_cidr", "192.168.1.0/24"},
		{&NetworkingStepDefinition, fieldGateway, "ip address"},
		{&NetworkingStepDefinition, "dns_servers", "comma-separated ip addresses"},
		{&NetworkingStepDefinition, "pod_cidr", "10.128.0.0/14"},
		{&NetworkingStepDefinition, "service_cidr", "172.30.0.0/16"},
		{&NetworkingStepDefinition, "start_ip", "192.168.1.140"},
		{&NetworkingStepDefinition, "vip", "auto-derive from static ip start"},
		{&AdvancedStepDefinition, "vm_id_base", "6000"},
		{&AdvancedStepDefinition, "ntp_server", "hostname or ip"},
		{&AdvancedStepDefinition, "ntp_server", "pool.ntp.org"},
		{&AdvancedStepDefinition, "bootstrap_timeout", "seconds"},
		{&AdvancedStepDefinition, "install_timeout", "seconds"},
		{&AdvancedStepDefinition, "terraform_env", "e.g. production"},
		{&AdvancedStepDefinition, "bin_dir", "absolute path"},
		{&FilesStepDefinition, "pull_secret", "path to"},
		{&FilesStepDefinition, "ssh_public_key", "path to"},
		{&ResourcesStepDefinition, "worker_vcpus", "okd minimum: 2 vcpus"},
		{&ResourcesStepDefinition, "worker_memory", "okd minimum: 8192 mb (8 gb)"},
	}

	for _, tc := range cases {
		fd := fieldByKey(t, tc.step, tc.key)
		if !strings.Contains(fd.Help, tc.token) {
			t.Errorf("%s.%s: Help %q does not state the shape token %q", tc.step.ID, tc.key, fd.Help, tc.token)
		}
	}
}

// TestProxmoxStep_TokenIDNamesTheRealEnvVar guards against the token_id
// field's Help ever citing PROXMOX_VE_API_TOKEN_SECRET, which does not
// exist anywhere in the codebase — the real env var it is combined with at
// deploy time is PROXMOX_VE_API_TOKEN in id=secret form (see
// internal/credentials/proxmox.go's envProxmoxAPIToken and
// internal/infrastructure/proxmox/probe.go's "must be in id=secret form").
func TestProxmoxStep_TokenIDNamesTheRealEnvVar(t *testing.T) {
	fd := fieldByKey(t, &ProxmoxStepDefinition, "token_id")
	if strings.Contains(fd.Help, "PROXMOX_VE_API_TOKEN_SECRET") {
		t.Errorf("token_id Help %q names PROXMOX_VE_API_TOKEN_SECRET, which does not exist in the codebase", fd.Help)
	}
	if !strings.Contains(fd.Help, "PROXMOX_VE_API_TOKEN") {
		t.Errorf("token_id Help %q does not name the real env var PROXMOX_VE_API_TOKEN", fd.Help)
	}
}

// TestResourcesStep_VCPUAndMemoryRegisterMatchesValidator asserts the
// control-plane and worker vcpus/memory fields use the same register
// ("okd minimum: N") for the hard floors OKD's config-level validation
// actually enforces (config.MinCPUControlPlaneOKD/MinCPUWorkerOKD and
// their memory counterparts), instead of the control-plane field reading
// as a hard floor while the worker field reads as a soft recommendation.
func TestResourcesStep_VCPUAndMemoryRegisterMatchesValidator(t *testing.T) {
	wantPrefix := map[string]string{
		"cp_vcpus":      "okd minimum: 4 vcpus",
		"worker_vcpus":  "okd minimum: 2 vcpus",
		"cp_memory":     "okd minimum: 8192 mb (8 gb)",
		"worker_memory": "okd minimum: 8192 mb (8 gb)",
	}
	for key, want := range wantPrefix {
		fd := fieldByKey(t, &ResourcesStepDefinition, key)
		if fd.Help != want {
			t.Errorf("resources.%s: Help = %q, want %q", key, fd.Help, want)
		}
	}
}

// TestAddonsStep_HelpIsLowercase asserts every addon field's Help row
// follows the wizard's lowercase, shape-first voice — product names
// ("1password", "vault", "bitwarden") and acronyms ("eso", "kv", "uuid",
// "url", "api") stay lowercase like the rest of the wizard's copy.
func TestAddonsStep_HelpIsLowercase(t *testing.T) {
	for _, sec := range AddonsStepDefinition.Sections {
		for i := range sec.Fields {
			fd := &sec.Fields[i]
			if fd.Help == "" {
				continue
			}
			if fd.Help != strings.ToLower(fd.Help) {
				t.Errorf("addons.%s: Help %q is not lowercase", fd.Key, fd.Help)
			}
		}
	}
}
