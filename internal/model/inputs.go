package model

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"maps"
	"slices"
	"strconv"

	"github.com/qxtaiba/okdctl/internal/config"
)

// Inputs is everything baked into the agent ISO that, if changed, makes the
// recorded cluster definition stale; Day1 names the nodes the ISO was built
// for, what Record.Nodes stores.
type Inputs struct {
	Inventory      Inventory
	Day1           []string
	Domain         string
	Network        config.NetworkingConfig
	VLAN           int
	Release        string
	PullSecret     []byte
	SSHKey         []byte
	ExtraManifests map[string][]byte
}

// Hash returns the sha256 hex of a canonical encoding of the ISO-baked
// inputs: the infra-ID, the identity of every master and Day1 worker, the
// VIPs, the domain, the machine and cluster networks, the release, the pull
// secret, the SSH key and the extra manifests. The encoding is pinned by
// TestInputsHash_Golden; changing it stales every record, so bump
// recordFile.Version with it.
func (in *Inputs) Hash() string {
	h := sha256.New()
	field := func(parts ...string) {
		for _, p := range parts {
			fmt.Fprintf(h, "%d:%s\n", len(p), p)
		}
	}
	field("infra", in.Inventory.InfraID)
	for i := range in.Inventory.Nodes {
		n := &in.Inventory.Nodes[i]
		if n.Role != RoleMaster && !slices.Contains(in.Day1, n.Name) {
			continue
		}
		field("node", string(n.Role), strconv.Itoa(n.Index), n.Name, strconv.Itoa(n.VMID), n.MAC, n.IP.String())
	}
	field("vip", in.Inventory.APIVIP.String(), in.Inventory.IngressVIP.String())
	net := &in.Network
	field("domain", in.Domain)
	field("machine", net.MachineCIDR, net.Gateway, strconv.Itoa(in.VLAN))
	list := func(label string, items []string) {
		field(append([]string{label, strconv.Itoa(len(items))}, items...)...)
	}
	list("dns", net.DNS)
	list("ntp", net.NTP)
	field("cluster-net", net.PodCIDR, net.ServiceCIDR, strconv.Itoa(net.HostPrefix))
	field("release", in.Release)
	field("pull-secret")
	writeBytes(h, in.PullSecret)
	field("ssh-key")
	writeBytes(h, in.SSHKey)
	for _, name := range slices.Sorted(maps.Keys(in.ExtraManifests)) {
		field("manifest", name)
		writeBytes(h, in.ExtraManifests[name])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// writeBytes streams b in the field encoding so the pull secret is never
// copied into a buffer nobody zeroizes.
func writeBytes(h hash.Hash, b []byte) {
	fmt.Fprintf(h, "%d:", len(b))
	h.Write(b)
	h.Write([]byte{'\n'})
}
