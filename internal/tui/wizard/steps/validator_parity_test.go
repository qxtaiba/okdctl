package steps

import (
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// wizardFieldParity maps a validated wizard field to a bad value and the
// cfg.Validate() field that must also reject it; loaderOnly marks shapes the
// yaml loader enforces at parse time, which Validate() never sees.
var wizardFieldParity = map[string]struct {
	bad        string
	wantField  string
	loaderOnly bool
}{
	config.FieldClusterName:           {bad: "Bad_Name", wantField: config.FieldClusterName},
	config.FieldClusterDomain:         {bad: "bad..domain", wantField: config.FieldClusterDomain},
	config.FieldProxmoxEndpoint:       {bad: "pve", wantField: config.FieldProxmoxEndpoint},
	config.FieldProxmoxVLAN:           {bad: "5000", wantField: config.FieldProxmoxVLAN},
	config.FieldProxmoxVMIDBase:       {bad: "5", wantField: config.FieldProxmoxVMIDBase},
	config.FieldNetworkingMachineCIDR: {bad: "10.0.0.0/40", wantField: config.FieldNetworkingMachineCIDR},
	config.FieldNetworkingGateway:     {bad: "999.1.1.1", wantField: config.FieldNetworkingGateway},
	config.FieldNetworkingDNS:         {bad: "999.1.1.1", wantField: config.FieldNetworkingDNS + "[0]"},
	config.FieldNetworkingAPIVIP:      {bad: "999.1.1.1", wantField: config.FieldNetworkingAPIVIP},
	config.FieldNetworkingIngressVIP:  {bad: "999.1.1.1", wantField: config.FieldNetworkingIngressVIP},
	config.FieldNetworkingNodeIPStart: {bad: "999.1.1.1", wantField: config.FieldNetworkingNodeIPStart},
	config.FieldNetworkingPodCIDR:     {bad: "10.128.0.0/40", wantField: config.FieldNetworkingPodCIDR},
	config.FieldNetworkingServiceCIDR: {bad: "not-a-cidr", wantField: config.FieldNetworkingServiceCIDR},
	fieldCPCPU:                        {bad: "1", wantField: fieldCPCPU},
	fieldCPMemory:                     {bad: "100", wantField: fieldCPMemory},
	fieldCPRootDisk:                   {bad: "10", wantField: fieldCPRootDisk},
	fieldWorkerCount:                  {bad: "-1", wantField: fieldWorkerCount},
	fieldWorkerCPU:                    {bad: "1", wantField: fieldWorkerCPU},
	fieldWorkerMemory:                 {bad: "100", wantField: fieldWorkerMemory},
	fieldWorkerRootDisk:               {bad: "10", wantField: fieldWorkerRootDisk},
	fieldWorkerDataDisk:               {bad: "-5", wantField: fieldWorkerDataDisk},
	config.FieldFilesPullSecret:       {bad: "", wantField: config.FieldFilesPullSecret},
	config.FieldFilesSSHPublicKey:     {bad: "", wantField: config.FieldFilesSSHPublicKey},
	fieldHookTimeout:                  {bad: "soon", loaderOnly: true},
	fieldDrainTimeout:                 {bad: "soon", loaderOnly: true},
}

func allStepDefinitions() []*wizard.StepDefinition {
	return []*wizard.StepDefinition{
		&BasicsStepDefinition,
		&ProxmoxStepDefinition,
		&NetworkingStepDefinition,
		&ResourcesStepDefinition,
		&FilesStepDefinition,
		&AdvancedStepDefinition,
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
				if entry.loaderOnly {
					continue
				}
				cfg := config.DefaultConfig()
				if err := fd.ConfigSet(cfg, entry.bad); err != nil {
					t.Errorf("ConfigSet(%q, %q): %v", fd.Key, entry.bad, err)
					continue
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
