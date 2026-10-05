// Package provision holds the ISO/ignition provisioning machinery shared by
// the setup phase and day-2 node operations: CoreOS ISO builds, Proxmox
// upload, the ignition HTTPS server, kernel-argument construction, and
// terraform.tfvars rendering.
package provision

import (
	"fmt"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

const workerIgnition = "worker.ign"

// IgnitionFilenames is the canonical list openshift-install emits into
// clusterDir and the ignition server deploys to the web root.
var IgnitionFilenames = []string{"bootstrap.ign", "master.ign", workerIgnition}

// Options carries the on-disk roots provisioning operations resolve artifacts from.
type Options struct {
	ProjectRoot string
	WorkDir     string
}

// NewOptions returns Options with WorkDir rooted at projectRoot.
func NewOptions(projectRoot string) Options {
	return Options{
		ProjectRoot: projectRoot,
		WorkDir:     workspace.WorkDir(projectRoot),
	}
}

// Provisioner drives the shared ISO/ignition provisioning operations.
type Provisioner struct {
	phase.BasePhase
	loggedISOs map[string]bool
}

// New constructs a Provisioner with the given base-phase options.
func New(opts ...phase.BasePhaseOption) *Provisioner {
	return &Provisioner{BasePhase: phase.NewBasePhase(opts...)}
}

// BuildIgnitionURL builds the base https:// URL where ignition payloads are
// served; Apache always binds port 443, so the port is never spelled out.
func BuildIgnitionURL(ip string) string {
	return fmt.Sprintf("https://%s/ignition", ip)
}

// CoreOSInfo describes a Fedora CoreOS download candidate resolved from the CoreOS stream metadata.
type CoreOSInfo struct {
	Version     string
	ISOUrl      string
	ISOChecksum string
}

// NodeInfo identifies a single VM emitted into the generated Terraform tfvars (role, IP).
type NodeInfo struct {
	Name string
	Role nodetypes.NodeRole
	IP   string
}
