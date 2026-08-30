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

// StepConfig declares one step in a wizard's sequence: its registry type
// and whether it is required (required steps cannot be skipped).
type StepConfig struct {
	Type     StepType
	Required bool
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
			{Type: StepTypeWelcome, Required: true},
			{Type: StepTypeDistribution, Required: true},
			{Type: StepTypeProxmox, Required: true},
			{Type: StepTypeBasics, Required: true},
			{Type: StepTypeNodePlacement, Required: false},
			{Type: StepTypeNetworking, Required: true},
			{Type: StepTypeResources, Required: true},
			{Type: StepTypeAddons, Required: false},
			{Type: StepTypeFiles, Required: true},
			{Type: StepTypeAdvanced, Required: false},
			{Type: StepTypeReview, Required: true},
		},
	}
}
