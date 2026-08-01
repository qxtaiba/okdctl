// Package workspace owns okdctl's on-disk workspace layout: the per-project
// work directory, the openshift-install cluster-config directory, the
// Terraform environment tree, and the sentinel filenames shared across
// phases. It is a dependency-light leaf so any package (system, config,
// phases, cli) can build these paths without layering hacks.
package workspace

import "path/filepath"

// WorkDirName is the per-project workdir okdctl creates under the project
// root for run artifacts (install-config, manifests, ignition, downloaded
// tools). system.isAllowedChownRoot reads it for the chown-back allowlist.
const WorkDirName = "okd-install"

// DefaultTerraformEnv names the embedded Terraform environment that ships in
// the binary and materializes on deploy.
const DefaultTerraformEnv = "production"

// BootstrapStateSentinelFile is the auto-loaded tfvars override postinstall
// writes after the bootstrap VM is destroyed. Terraform loads *.auto.tfvars.json
// after terraform.tfvars, so cleanup and setup must remove this file before any
// subsequent deploy so bootstrap_enabled=true takes effect again.
const BootstrapStateSentinelFile = "bootstrap-state.auto.tfvars.json"

// WorkDir returns the per-project work directory (<projectRoot>/okd-install).
func WorkDir(projectRoot string) string {
	return filepath.Join(projectRoot, WorkDirName)
}

// ClusterConfigDir returns the openshift-install output directory
// (<workDir>/cluster-config) holding install-config.yaml and the generated
// kubeconfig/auth bundle.
func ClusterConfigDir(workDir string) string {
	return filepath.Join(workDir, "cluster-config")
}

// TerraformEnvDir returns the Terraform environment directory
// (<projectRoot>/infrastructure/terraform/environments/<env>) for env. An
// empty env resolves to the environments directory itself.
func TerraformEnvDir(projectRoot, env string) string {
	return filepath.Join(projectRoot, "infrastructure", "terraform", "environments", env)
}

// TerraformModuleDir returns the materialized proxmox-okd module directory
// (<projectRoot>/infrastructure/terraform/modules/proxmox-okd). The
// terraform.Executor's stale-override guard derives the same path relative
// to TerraformEnvDir; the two must stay in lockstep.
func TerraformModuleDir(projectRoot string) string {
	return filepath.Join(projectRoot, "infrastructure", "terraform", "modules", "proxmox-okd")
}

// KubeconfigPath returns the admin kubeconfig path
// (<clusterDir>/auth/kubeconfig) openshift-install writes.
func KubeconfigPath(clusterDir string) string {
	return filepath.Join(clusterDir, "auth", "kubeconfig")
}
