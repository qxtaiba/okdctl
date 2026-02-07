// Package config provides configuration management for the CLI.
package config

// DistributionType represents supported Kubernetes distributions.
type DistributionType string

const (
	DistributionOKD DistributionType = "okd"
)

// ProviderType represents supported infrastructure providers.
type ProviderType string

const (
	ProviderProxmox ProviderType = "proxmox"
)

// SupportedDistributions returns a list of all supported Kubernetes distributions.
func SupportedDistributions() []DistributionType {
	return []DistributionType{
		DistributionOKD,
	}
}

// SupportedProviders returns a list of all supported infrastructure providers.
func SupportedProviders() []ProviderType {
	return []ProviderType{
		ProviderProxmox,
	}
}
