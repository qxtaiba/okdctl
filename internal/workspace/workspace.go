// Package workspace owns okdctl's on-disk workspace layout: the per-project
// work directory and the openshift-install cluster-config directory. It is a
// dependency-light leaf so any package can build these paths without
// layering hacks.
package workspace

import "path/filepath"

// WorkDirName is the per-project workdir for run artifacts (install-config,
// manifests, ignition, downloaded tools); system.isAllowedChownRoot reads
// it for the chown-back allowlist.
const WorkDirName = "okd-install"

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

// KubeconfigPath returns the admin kubeconfig path
// (<clusterDir>/auth/kubeconfig) openshift-install writes.
func KubeconfigPath(clusterDir string) string {
	return filepath.Join(clusterDir, "auth", "kubeconfig")
}
