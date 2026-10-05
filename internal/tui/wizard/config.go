package wizard

import (
	"github.com/qxtaiba/okdctl/internal/config"
)

// Config is the declarative description of a wizard: which steps run, the
// seed Config, and whether an existing okdctl.yaml is present.
type Config struct {
	Steps         []StepConfig
	InitialConfig *config.Config
	ConfigExists  bool
}

// StepConfig declares sequence order; visible steps must validate before advancing.
type StepConfig struct {
	Type StepType
}

// StepType names an entry in the StepBuilder factory registry, distinct
// from StepID (step.go) which identifies an already-constructed WizardStep
// at runtime.
type StepType string

// Built-in StepType values in the default StepBuilder factory registry.
const (
	StepTypeWelcome       StepType = "welcome"
	StepTypeDistribution  StepType = "distribution"
	StepTypeProxmox       StepType = "proxmox"
	StepTypeBasics        StepType = "basics"
	StepTypeNodePlacement StepType = "node-placement"
	StepTypeNetworking    StepType = "networking"
	StepTypeResources     StepType = "resources"
	StepTypeAddons        StepType = "addons"
	StepTypeFiles         StepType = "files"
	StepTypeAdvanced      StepType = "advanced"
	StepTypeReview        StepType = "review"
)

// DefaultConfig returns the step sequence used by "okdctl configure".
func DefaultConfig() Config {
	return Config{
		Steps: []StepConfig{
			{Type: StepTypeWelcome},
			{Type: StepTypeDistribution},
			{Type: StepTypeProxmox},
			{Type: StepTypeBasics},
			{Type: StepTypeNodePlacement},
			{Type: StepTypeNetworking},
			{Type: StepTypeResources},
			{Type: StepTypeAddons},
			{Type: StepTypeFiles},
			{Type: StepTypeAdvanced},
			{Type: StepTypeReview},
		},
	}
}
