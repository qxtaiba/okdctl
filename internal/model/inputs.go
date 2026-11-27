package model

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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

// Hash returns the sha256 hex of a canonical encoding of the infra-ID, the
// identity (role, index, name, VMID, MAC, IP) of every master and of every
// worker named in Day1, the VIPs, the domain, the machine network (CIDR,
// gateway, resolvers, NTP, VLAN), the cluster CIDRs, the release, the pull
// secret, the SSH key and the extra manifests by sorted name. A worker added after day 1 stays
// out; a removed day-1 worker drops out and changes the digest, so the step
// that removes one rewrites the record; sizing and placement are excluded
// because the driver reconciles them.
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
	field("pull-secret", string(in.PullSecret))
	field("ssh-key", string(in.SSHKey))
	for _, name := range slices.Sorted(maps.Keys(in.ExtraManifests)) {
		field("manifest", name, string(in.ExtraManifests[name]))
	}
	sum := sha256.Sum256(buf.Bytes())
	return hex.EncodeToString(sum[:])
}
