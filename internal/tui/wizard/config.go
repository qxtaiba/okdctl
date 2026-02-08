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
