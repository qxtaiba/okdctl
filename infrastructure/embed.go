// Package infrastructure embeds the Terraform sources that okdctl
// materializes into a workspace, so a packaged binary can deploy from any
// empty directory without a source checkout.
package infrastructure

import "embed"

// TerraformFS holds the deployable Terraform tree: the proxmox-okd module
// and the production environment, including the committed provider lock
// file. Paths are relative to this directory. Runtime artifacts (tfvars,
// tfstate, .terraform/) are deliberately absent; the embed list is explicit
// so a dirty dev checkout can never leak them into the binary.
// TestEmbeddedTerraformMatchesDisk fails when a new source file under
// terraform/modules or terraform/environments is missing from this list.
//
//go:embed terraform/modules/proxmox-okd/main.tf
//go:embed terraform/modules/proxmox-okd/ha.tf
//go:embed terraform/modules/proxmox-okd/variables.tf
//go:embed terraform/modules/proxmox-okd/output.tf
//go:embed terraform/modules/proxmox-okd/versions.tf
//go:embed terraform/environments/production/main.tf
//go:embed terraform/environments/production/variables.tf
//go:embed terraform/environments/production/outputs.tf
//go:embed terraform/environments/production/versions.tf
//go:embed terraform/environments/production/.terraform.lock.hcl
//go:embed terraform/environments/production/.gitignore
var TerraformFS embed.FS
