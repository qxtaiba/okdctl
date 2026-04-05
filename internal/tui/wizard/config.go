package wizard

import (
	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
)

type Config struct {
	Steps         []StepConfig
	InitialConfig *config.Config
	ConfigExists  bool
}

type StepConfig struct {
	Type     StepType
	Required bool
}

// StepType names an entry in the StepBuilder factory registry. It is the
// external, declarative identifier used when assembling a wizard from a Config
// (see DefaultConfig). It is distinct from StepID (see step.go), which
// identifies an already-constructed WizardStep instance at runtime.
type StepType string

const (
	StepTypeWelcome      StepType = "welcome"
	StepTypeDistribution StepType = "distribution"
	StepTypeProxmox      StepType = "proxmox"
	StepTypeBasics       StepType = "basics"
	StepTypeNetworking   StepType = "networking"
	StepTypeResources    StepType = "resources"
	StepTypeAddons       StepType = "addons"
	StepTypeFiles        StepType = "files"
	StepTypeAdvanced     StepType = "advanced"
	StepTypeReview       StepType = "review"
)

func DefaultConfig() Config {
	return Config{
		Steps: []StepConfig{
			{Type: StepTypeWelcome, Required: true},
			{Type: StepTypeDistribution, Required: true},
			{Type: StepTypeProxmox, Required: true},
			{Type: StepTypeBasics, Required: true},
			{Type: StepTypeNetworking, Required: true},
			{Type: StepTypeResources, Required: true},
			{Type: StepTypeAddons, Required: false},
			{Type: StepTypeFiles, Required: true},
			{Type: StepTypeAdvanced, Required: false},
			{Type: StepTypeReview, Required: true},
		},
	}
}
