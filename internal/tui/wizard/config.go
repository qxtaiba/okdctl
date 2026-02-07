package wizard

import (
	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
)

// ===============================================================================
// WIZARD CONFIGURATION
// ===============================================================================

// Config configures which steps appear in the wizard and their order.
type Config struct {
	Steps         []StepConfig
	InitialConfig *config.Config
	ConfigExists  bool
}

// StepConfig configures an individual wizard step.
type StepConfig struct {
	Type     StepType
	Required bool
}

// StepType identifies a wizard step type.
type StepType string

const (
	StepTypeWelcome      StepType = "welcome"
	StepTypeDistribution StepType = "distribution"
	StepTypeProxmox      StepType = "proxmox"
	StepTypeBasics       StepType = "basics"
	StepTypeNetworking   StepType = "networking"
	StepTypeResources    StepType = "resources"
	StepTypeFeatures     StepType = "features"
	StepTypeFiles        StepType = "files"
	StepTypeAdvanced     StepType = "advanced"
	StepTypeReview       StepType = "review"
)

// DefaultConfig returns the standard wizard configuration with all steps.
func DefaultConfig() Config {
	return Config{
		Steps: []StepConfig{
			{Type: StepTypeWelcome, Required: true},
			{Type: StepTypeDistribution, Required: true},
			{Type: StepTypeProxmox, Required: true},
			{Type: StepTypeBasics, Required: true},
			{Type: StepTypeNetworking, Required: true},
			{Type: StepTypeResources, Required: true},
			{Type: StepTypeFeatures, Required: false},
			{Type: StepTypeFiles, Required: true},
			{Type: StepTypeAdvanced, Required: false},
			{Type: StepTypeReview, Required: true},
		},
	}
}
