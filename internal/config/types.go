package config

// DistributionType identifies the Kubernetes distribution to deploy.
// Scaffolding: single-variant enum so a future variant needs no call-site changes.
type DistributionType string

// Distributions okdctl can deploy.
const (
	DistributionOKD DistributionType = "okd"
)

// ProviderType identifies the infrastructure provider.
// Scaffolding: single-variant enum for call-site stability, not future expansion.
type ProviderType string

// Providers okdctl can deploy onto.
const (
	ProviderProxmox ProviderType = "proxmox"
)
