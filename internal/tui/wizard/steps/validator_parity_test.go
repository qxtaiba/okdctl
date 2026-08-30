package steps

import (
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// wizardFieldParity maps a validated wizard field to a bad value and the
// file-load field that must also reject it.
var wizardFieldParity = map[string]struct {
	bad       string
	wantField string
}{
	"cluster_name":      {"a", config.FieldClusterName},
	"domain":            {"ab", config.FieldClusterDomain},
	"worker_count":      {"101", config.FieldTopologyWorkersCount},
	"machine_cidr":      {"10.0.0.0/40", config.FieldNetworkingMachineCIDR},
	"gateway":           {"999.1.1.1", config.FieldNetworkingGateway},
	"dns_servers":       {"999.1.1.1", config.FieldNetworkingDNS + "[0]"},
	"pod_cidr":          {"10.128.0.0/40", config.FieldNetworkingPodCIDR},
	"service_cidr":      {"not-a-cidr", config.FieldNetworkingServiceCIDR},
	"start_ip":          {"999.1.1.1", config.FieldNetworkingStaticIPStart},
	"bastion_ip":        {"999.1.1.1", config.FieldNetworkingBastionIP},
	"vip":               {"999.1.1.1", "networking.bastion.vip"},
	"ntp_server":        {"!bad host!", config.FieldNetworkingNTPServer},
	"host":              {"-bad.host", config.FieldProxmoxHost},
	"pull_secret":       {"/nonexistent/pull-secret.json", config.FieldFilesPullSecret},
	"ssh_public_key":    {"/nonexistent/key.pub", config.FieldFilesSSHPublicKey},
	"cp_vcpus":          {"129", config.FieldTopologyControlPlaneCPU},
	"cp_memory":         {"2000000", config.FieldTopologyControlPlaneMemory},
	"cp_disk":           {"1001", config.FieldTopologyControlPlaneDisk},
	"worker_vcpus":      {"129", config.FieldTopologyWorkersCPU},
	"worker_memory":     {"2000000", config.FieldTopologyWorkersMemory},
	"worker_disk":       {"1001", config.FieldTopologyWorkersDisk},
	"worker_data_disk":  {"5001", config.FieldDisksWorkerDataSize},
	"cp_data_disk":      {"5001", config.FieldDisksControlPlaneDataSize},
	"vm_id_base":        {"5", config.FieldTopologyVMIDBase},
	"bootstrap_timeout": {"10", config.FieldDeploymentBootstrapTimeout},
	"install_timeout":   {"10", config.FieldDeploymentInstallTimeout},
	"terraform_env":     {"../evil", config.FieldDeploymentTerraformEnv},
	"bin_dir":           {"relative/bin", config.FieldDeploymentBinDir},
}

func allStepDefinitions() []*wizard.StepDefinition {
	return []*wizard.StepDefinition{
		&BasicsStepDefinition,
		&NetworkingStepDefinition,
		&ProxmoxStepDefinition,
		&ResourcesStepDefinition,
		&FilesStepDefinition,
		&AdvancedStepDefinition,
		&AddonsStepDefinition,
	}
}

func TestWizardFieldValidatorsHaveFileLoadParity(t *testing.T) {
	hasField := func(r *config.ValidationResult, field string) bool {
		for _, e := range r.Errors {
			if e.Field == field {
				return true
			}
		}
		return false
	}

	seen := map[string]bool{}
	for _, def := range allStepDefinitions() {
		for _, sec := range def.Sections {
			for _, fd := range sec.Fields {
				if fd.Validate == nil {
					continue
				}
				seen[fd.Key] = true
				entry, ok := wizardFieldParity[fd.Key]
				if !ok {
					t.Errorf("wizard field %q has a validator but no entry in wizardFieldParity; add its file-load mapping", fd.Key)
					continue
				}
				if err := fd.Validate(entry.bad); err == nil {
					t.Errorf("wizard validator for %q accepts the parity table's bad value %q", fd.Key, entry.bad)
					continue
				}
				cfg := config.DefaultConfig()
				if fd.ConfigSet != nil {
					if err := fd.ConfigSet(cfg, entry.bad); err != nil {
						t.Errorf("ConfigSet(%q, %q): %v", fd.Key, entry.bad, err)
						continue
					}
				}
				if result := cfg.Validate(); !hasField(result, entry.wantField) {
					t.Errorf("file-load validation misses %q after wizard field %q is set to %q; errors: %v",
						entry.wantField, fd.Key, entry.bad, result.Errors)
				}
			}
		}
	}

	for key := range wizardFieldParity {
		if !seen[key] {
			t.Errorf("wizardFieldParity entry %q matches no validated wizard field; remove or fix the key", key)
		}
	}
}
