// Package workspace owns okdctl's v3 on-disk layout under a workspace root:
// cluster/ (record, rendered installer inputs, auth, state, ISO), handoff/,
// backups/ and bin/.
package workspace

import "path/filepath"

// Layout names pin the on-disk contract: renaming one orphans existing
// workspaces.
const (
	ClusterDirName = "cluster"
	HandoffDirName = "handoff"
	BackupsDirName = "backups"
	BinDirName     = "bin"
	RecordFile     = "record.json"
	ISOFile        = "agent.x86_64.iso"
)

// ClusterDir returns <root>/cluster, created by up and removed by destroy.
func ClusterDir(root string) string {
	return filepath.Join(root, ClusterDirName)
}

// RecordPath returns the cluster record (infra-ID, inputs hash, release).
func RecordPath(root string) string {
	return filepath.Join(ClusterDir(root), RecordFile)
}

// AuthDir returns the directory openshift-install writes credentials into.
func AuthDir(root string) string {
	return filepath.Join(ClusterDir(root), "auth")
}

// KubeconfigPath returns the admin kubeconfig path.
func KubeconfigPath(root string) string {
	return filepath.Join(AuthDir(root), "kubeconfig")
}

// KubeadminPasswordPath returns the kubeadmin password file path.
func KubeadminPasswordPath(root string) string {
	return filepath.Join(AuthDir(root), "kubeadmin-password")
}

// StateFilePath returns the installer state file, which holds signer private
// keys and tokens and must stay 0600.
func StateFilePath(root string) string {
	return filepath.Join(ClusterDir(root), ".openshift_install_state.json")
}

// ISOPath returns the agent ISO path, deleted after install-complete.
func ISOPath(root string) string {
	return filepath.Join(ClusterDir(root), ISOFile)
}

// ExtraManifestsDir returns the directory of extra manifests baked into the ISO.
func ExtraManifestsDir(root string) string {
	return filepath.Join(ClusterDir(root), "openshift")
}

// HandoffManifestsDir returns the operator-provided manifests directory.
func HandoffManifestsDir(root string) string {
	return filepath.Join(root, HandoffDirName, "manifests")
}

// HandoffSecretsDir returns the operator-provided secrets directory, which
// must stay 0700 and gitignored.
func HandoffSecretsDir(root string) string {
	return filepath.Join(root, HandoffDirName, "secrets")
}

// BackupsDir returns the parent of the per-timestamp backup directories.
func BackupsDir(root string) string {
	return filepath.Join(root, BackupsDirName)
}

// BinDir returns the directory of pinned tools (openshift-install, oc, nmstatectl).
func BinDir(root string) string {
	return filepath.Join(root, BinDirName)
}
