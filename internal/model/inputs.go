package model

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"slices"
	"strconv"
)

// Inputs is everything baked into the agent ISO that, if changed, makes the
// recorded cluster definition stale.
type Inputs struct {
	Inventory      Inventory
	Release        string
	PullSecret     []byte
	SSHKey         []byte
	ExtraManifests map[string][]byte
}

// Hash returns the sha256 hex of a canonical encoding of the node identities
// (role, index, name, VMID, MAC, IP), the VIPs, the release, the pull secret,
// the SSH key and the extra manifests by sorted name; sizing and placement
// are excluded because the driver reconciles them.
func (in *Inputs) Hash() string {
	var buf bytes.Buffer
	field := func(parts ...string) {
		for _, p := range parts {
			fmt.Fprintf(&buf, "%d:%s\n", len(p), p)
		}
	}
	field("infra", in.Inventory.InfraID)
	for i := range in.Inventory.Nodes {
		n := &in.Inventory.Nodes[i]
		field("node", string(n.Role), strconv.Itoa(n.Index), n.Name, strconv.Itoa(n.VMID), n.MAC, n.IP.String())
	}
	field("vip", in.Inventory.APIVIP.String(), in.Inventory.IngressVIP.String())
	field("release", in.Release)
	field("pull-secret", string(in.PullSecret))
	field("ssh-key", string(in.SSHKey))
	for _, name := range slices.Sorted(maps.Keys(in.ExtraManifests)) {
		field("manifest", name, string(in.ExtraManifests[name]))
	}
	sum := sha256.Sum256(buf.Bytes())
	return hex.EncodeToString(sum[:])
}
