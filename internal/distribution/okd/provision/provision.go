// Package provision holds the ISO/ignition provisioning machinery shared by
// the setup phase and day-2 node operations: CoreOS ISO resolution and
// custom-ISO builds, Proxmox ISO upload, the ignition HTTPS server (apache
// vhost, TLS cert), kernel-argument construction, node-list derivation, and
// terraform.tfvars rendering.
package provision

import (
	"fmt"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/platform"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

// IgnitionFilenames is the canonical list openshift-install emits into
// clusterDir and the ignition server deploys into the web root.
var IgnitionFilenames = []string{"bootstrap.ign", "master.ign", "worker.ign"}

// Options carries the on-disk roots provisioning operations resolve
// artifacts from.
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

// Provisioner drives the shared ISO/ignition provisioning operations. Host
// OS detection populates OS; detection errors fall back to RHEL defaults.
type Provisioner struct {
	phase.BasePhase
	OS         platform.OS
	loggedISOs map[string]bool
}

// New constructs a Provisioner with the given base-phase options.
func New(opts ...phase.BasePhaseOption) *Provisioner {
	bp := phase.NewBasePhase(opts...)
	return &Provisioner{
		BasePhase: bp,
		OS:        platform.DetectOrDefault(bp.Log),
	}
}

// BuildIgnitionURL builds the base https:// URL where ignition payloads are
// served. Apache always binds port 443 (see ConfigureApache), so the port is
// never spelled in the URL.
func BuildIgnitionURL(ip string) string {
	return fmt.Sprintf("https://%s/ignition", ip)
}

// CoreOSInfo describes a Fedora CoreOS download candidate resolved from
// the CoreOS stream metadata.
type CoreOSInfo struct {
	Version      string
	ISOUrl       string
	ISOChecksum  string
	Architecture string
}

// NodeInfo identifies a single VM emitted into the generated Terraform
// tfvars (role, IP, MAC).
type NodeInfo struct {
	Name string
	Role nodetypes.NodeRole
	IP   string
	MAC  string
}
