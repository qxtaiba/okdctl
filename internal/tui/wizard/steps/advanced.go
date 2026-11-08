// Package steps holds the wizard's configure-flow screens.
package steps

import (
	"errors"
	"net/netip"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// AdvancedStepDefinition declares the cluster-network, lifecycle and install
// option fields.
var AdvancedStepDefinition = wizard.StepDefinition{
	ID:           wizard.StepIDAdvanced,
	Title:        "advanced settings",
	DisplayTitle: "advanced settings",
	Description:  "cluster networks, node lifecycle and install options",
	Sections: []wizard.SectionDefinition{
		{
			Title: "cluster networks",
			Fields: []wizard.FieldDefinition{
				{
					Key:       config.FieldNetworkingPodCIDR,
					Label:     "pod cidr",
					Default:   "10.128.0.0/14",
					Help:      "kubernetes pod network (okd default: 10.128.0.0/14)",
					Required:  true,
					Validate:  validateCIDR,
					ConfigSet: wizard.SetString(func(c *config.Config, v string) { c.Networking.PodCIDR = v }),
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.Networking.PodCIDR }),
				},
				{
					Key:       config.FieldNetworkingServiceCIDR,
					Label:     "service cidr",
					Default:   "172.30.0.0/16",
					Help:      "kubernetes service network (okd default: 172.30.0.0/16)",
					Required:  true,
					Validate:  validateCIDR,
					ConfigSet: wizard.SetString(func(c *config.Config, v string) { c.Networking.ServiceCIDR = v }),
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.Networking.ServiceCIDR }),
				},
				{
					Key:       config.FieldNetworkingHostPrefix,
					Label:     "host prefix",
					Default:   "23",
					Help:      "pod subnet size per node (smaller = more pods)",
					Type:      wizard.FieldTypeSelect,
					Options:   []string{"20", "21", "22", "23", "24", "25", "26", "27", "28"},
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.Networking.HostPrefix = v }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.Networking.HostPrefix }),
				},
			},
		},
		{
			Title: "node lifecycle",
			Fields: []wizard.FieldDefinition{
				{
					Key:         fieldHookCommand,
					Label:       "before next node",
					Placeholder: "none",
					Help:        "command run between rolling node operations — blank disables the hook",
					Width:       wizard.FieldWidthPath,
					ConfigSet:   wizard.SetString(func(c *config.Config, v string) { c.Lifecycle.BeforeNextNode.Command = v }),
					ConfigGet:   wizard.GetString(func(c *config.Config) string { return c.Lifecycle.BeforeNextNode.Command }),
				},
				{
					Key:       fieldHookTimeout,
					Label:     "hook timeout",
					Default:   "15m",
					Help:      "how long the hook may run, in go duration syntax such as 15m",
					Width:     wizard.FieldWidthNumber,
					Required:  true,
					Validate:  validateDuration,
					ConfigSet: setDuration(func(c *config.Config, d time.Duration) { c.Lifecycle.BeforeNextNode.Timeout = d }),
					ConfigGet: getDuration(func(c *config.Config) time.Duration { return c.Lifecycle.BeforeNextNode.Timeout }),
				},
				{
					Key:       fieldDrainTimeout,
					Label:     "drain timeout",
					Default:   "10m",
					Help:      "how long a node drain may take, in go duration syntax such as 10m",
					Width:     wizard.FieldWidthNumber,
					Required:  true,
					Validate:  validateDuration,
					ConfigSet: setDuration(func(c *config.Config, d time.Duration) { c.Lifecycle.DrainTimeout = d }),
					ConfigGet: getDuration(func(c *config.Config) time.Duration { return c.Lifecycle.DrainTimeout }),
				},
			},
		},
		{
			Title: "install options",
			Fields: []wizard.FieldDefinition{
				{
					Key:       fieldHandoff,
					Label:     "apply handoff",
					Default:   valYes,
					Help:      "apply handoff/secrets and handoff/manifests once the install completes",
					Type:      wizard.FieldTypeSelect,
					Options:   []string{valNo, valYes},
					ConfigSet: wizard.SetBool(func(c *config.Config, v bool) { c.Handoff.Enabled = v }),
					ConfigGet: getBool(func(c *config.Config) bool { return c.Handoff.Enabled }),
				},
				{
					Key:       fieldCatalogs,
					Label:     "disable red hat catalogs",
					Default:   valYes,
					Help:      "turn off the red hat operator catalogs that need a subscription",
					Type:      wizard.FieldTypeSelect,
					Options:   []string{valNo, valYes},
					ConfigSet: wizard.SetBool(func(c *config.Config, v bool) { c.Defaults.DisableRedHatCatalogs = v }),
					ConfigGet: getBool(func(c *config.Config) bool { return c.Defaults.DisableRedHatCatalogs }),
				},
			},
		},
	},
	Validate: func(values map[string]string) error {
		pod, podErr := netip.ParsePrefix(values[config.FieldNetworkingPodCIDR])
		service, serviceErr := netip.ParsePrefix(values[config.FieldNetworkingServiceCIDR])
		if podErr == nil && serviceErr == nil && pod.Overlaps(service) {
			return wizard.NewCrossFieldError(
				errors.New("pod cidr and service cidr must not overlap — widen or move one of the ranges"),
				config.FieldNetworkingPodCIDR, config.FieldNetworkingServiceCIDR)
		}
		return nil
	},
}

// NewAdvancedStep returns the advanced-settings wizard step.
func NewAdvancedStep() *wizard.DataDrivenStep {
	return wizard.NewDataDrivenStep(&AdvancedStepDefinition)
}
