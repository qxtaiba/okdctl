package wizard

import (
	"github.com/qxtaiba/okdctl/internal/config"
)

// Config is the declarative description of a wizard.
type Config struct {
	Steps         []StepConfig
	InitialConfig *config.Config
	ConfigExists  bool
}

// StepConfig declares sequence order; visible steps must validate before
// advancing.
type StepConfig struct {
	Type StepType
}

// StepType names a StepBuilder registry entry; StepID identifies a built
// step at runtime.
type StepType string

// Built-in StepType values.
const (
	StepTypeWelcome       StepType = "welcome"
	StepTypeDistribution  StepType = "distribution"
	StepTypeProxmox       StepType = "proxmox"
	StepTypeBasics        StepType = "basics"
	StepTypeNodePlacement StepType = "node-placement"
	StepTypeNetworking    StepType = "networking"
	StepTypeResources     StepType = "resources"
	StepTypeFiles         StepType = "files"
	StepTypeAdvanced      StepType = "advanced"
	StepTypeReview        StepType = "review"
)

// DefaultConfig returns the configure wizard's step sequence.
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
			{Type: StepTypeFiles},
			{Type: StepTypeAdvanced},
			{Type: StepTypeReview},
		},
	}
}
