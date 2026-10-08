package steps

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/luthermonson/go-proxmox"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/httputil"
	infraproxmox "github.com/qxtaiba/okdctl/internal/infrastructure/proxmox"
)

const proxmoxStatusOnline = "online"

type proxmoxNode struct {
	Name         string
	Status       string // "online" or "offline"
	CPUs         int
	CPUsKnown    bool
	MemGB        int
	MemKnown     bool
	Storage      []proxmoxStorage
	StorageKnown bool
	Bridges      []proxmoxBridge
	BridgesKnown bool
}

type proxmoxStorage struct {
	Name       string
	Content    string // comma-separated: images, iso, backup, etc.
	TotalGB    int
	TotalKnown bool
}

type proxmoxBridge struct {
	Name string
	CIDR string // e.g. "192.168.1.1/24" if configured
}

type proxmoxDiscovery struct {
	Nodes   []proxmoxNode
	Storage []proxmoxStorage
	Bridges []proxmoxBridge

	// Heterogeneous reports that online nodes' inventories differed, so
	// Storage/Bridges hold only what every online node shares and the
	// placement step should say so.
	Heterogeneous bool
}

// discoverProxmox queries every online Proxmox node — not just one sampled
// node — so Storage/Bridges reflect what every node actually shares,
// and cross-checks the config's already-chosen placement (storage/bridge
// per role) against each node's own reported inventory: a selection valid
// when it was made can go stale the moment discovery reports a node that
// doesn't actually carry it. parent bounds the whole fetch to the current
// wizard visit — leaving the step cancels any request still in flight.
func discoverProxmox(parent context.Context, cfg *config.Config) (*proxmoxDiscovery, error) {
	if cfg.Provider.Proxmox == nil {
		return nil, fmt.Errorf("no proxmox config")
	}
	px := cfg.Provider.Proxmox
	switch {
	case px.Host == "" || px.Username == "":
		return nil, fmt.Errorf("missing credentials — enter host and username in the proxmox step")
	case px.Password.IsEmpty() && px.TokenID != "":
		return nil, fmt.Errorf("discovery uses password auth — enter a password in the proxmox step (token id is saved for deploy)")
	case px.Password.IsEmpty():
		return nil, fmt.Errorf("missing credentials — enter host, username, and password in the proxmox step")
	}

	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()

	httpClient := httputil.NewOptionalInsecure(px.Insecure, 10*time.Second)

	client := proxmox.NewClient(
		infraproxmox.APIBaseURL(px.Host),
		proxmox.WithHTTPClient(httpClient),
		proxmox.WithCredentials(&proxmox.Credentials{Username: px.Username, Password: string(px.Password.Bytes())}),
	)

	rawNodes, err := client.Nodes(ctx)
	if err != nil {
		return nil, classifyError(err)
	}
	if len(rawNodes) == 0 {
		return nil, fmt.Errorf("no nodes found in cluster — check that the proxmox cluster has at least one node")
	}

	nodes := make([]proxmoxNode, 0, len(rawNodes))
	for _, n := range rawNodes {
		nodes = append(nodes, proxmoxNode{
			Name:      n.Node,
			Status:    n.Status,
			CPUs:      n.MaxCPU,
			CPUsKnown: true,
			MemGB:     int(n.MaxMem / (1024 * 1024 * 1024)), //nolint:gosec // G115: uint64→int is safe for GB-scale memory
			MemKnown:  true,
		})
	}

	online := make([]string, 0, len(nodes))
	for i := range nodes {
		n := &nodes[i]
		if n.Status == proxmoxStatusOnline {
			online = append(online, n.Name)
		}
	}
	if len(online) == 0 {
		online = []string{nodes[0].Name}
	}

	storage, bridges, heterogeneous, inventories := fetchClusterDetails(ctx, client, online)
	for i := range nodes {
		if inventory, ok := inventories[nodes[i].Name]; ok {
			nodes[i].Storage = inventory.Storage
			nodes[i].StorageKnown = inventory.StorageKnown
			nodes[i].Bridges = inventory.Bridges
			nodes[i].BridgesKnown = inventory.BridgesKnown
		}
	}

	disc := &proxmoxDiscovery{
		Nodes:         nodes,
		Storage:       storage,
		Bridges:       bridges,
		Heterogeneous: heterogeneous,
	}

	// Each node's OWN full inventory (not the cross-node intersection
	// above) is what placement validity actually depends on — a storage
	// pool missing from the shared set can still be exactly right for the
	// one node a role is pinned to.
	placementInventory := make(map[string]config.ProxmoxNodeInventory, len(inventories))
	for name, inv := range inventories {
		placementInventory[name] = config.ProxmoxNodeInventory{
			Storage: storageNames(inv.Storage),
			Bridges: bridgeNames(inv.Bridges),
		}
	}
	if result := config.ValidatePlacementAgainstInventory(cfg, placementInventory); !result.IsValid() {
		return disc, errors.New(result.Error())
	}

	return disc, nil
}

// startDiscovery resets the step into the discovering phase and issues a
// generation-tagged discovery fetch over a snapshotted cfg, so the returned
// tea.Cmd never touches s once it is handed to bubbletea. The Proxmox
// password is cloned via SetBytes rather than a string hop — a string copy
// would be immutable and unzeroizable for the process lifetime — into a
// detached SecretBytes the closure (and only the closure) zeroizes once the
// fetch finishes; ownedPasswords tracks it so Release can still zeroize it
// if the UI exits before the fetch completes.
func (s *NodePlacementStep) startDiscovery() tea.Cmd {
	s.phase = phaseDiscovering
	s.discoveryErr = nil
	s.generation++
	generation := s.generation
	parent := s.Context()
	cfg := *s.cfg
	px := *cfg.Provider.Proxmox
	px.Password = config.SecretBytes{}
	px.Password.SetBytes(cfg.Provider.Proxmox.Password.Bytes())
	cfg.Provider.Proxmox = &px
	s.ownedPasswords = append(s.ownedPasswords, &px.Password)
	return func() tea.Msg {
		defer px.Password.Zeroize()
		disc, err := discoverProxmox(parent, &cfg)
		return discoveryCompleteMsg{generation: generation, discovery: disc, err: err}
	}
}

// fetchClusterDetails pulls storage/bridges from every online node and
// keeps only what all of them share (by name), so the wizard's single
// cluster-wide pick lists never offer a resource missing on the node a VM
// lands on; heterogeneous reports whether any two inventories differed. A
// category whose fetch failed on a node (nil, best-effort) neither narrows
// the result nor counts as a difference.
type proxmoxNodeInventory struct {
	Storage      []proxmoxStorage
	StorageKnown bool
	Bridges      []proxmoxBridge
	BridgesKnown bool
}

func fetchClusterDetails(ctx context.Context, client *proxmox.Client, nodeNames []string) (storage []proxmoxStorage, bridges []proxmoxBridge, heterogeneous bool, inventories map[string]proxmoxNodeInventory) {
	inventories = make(map[string]proxmoxNodeInventory, len(nodeNames))
	first := fetchNodeDetails(ctx, client, nodeNames[0])
	inventories[nodeNames[0]] = first
	storage, bridges = first.Storage, first.Bridges

	for _, nodeName := range nodeNames[1:] {
		details := fetchNodeDetails(ctx, client, nodeName)
		inventories[nodeName] = details
		var differs bool
		storage, differs = keepShared(storage, details.Storage, func(v proxmoxStorage) string { return v.Name })
		heterogeneous = heterogeneous || differs
		bridges, differs = keepShared(bridges, details.Bridges, func(v proxmoxBridge) string { return v.Name })
		heterogeneous = heterogeneous || differs
	}

	return storage, bridges, heterogeneous, inventories
}

// keepShared returns the elements of base whose key other also has, plus
// whether the two sets differed at all; a nil side (that category's fetch
// errored on that node) never narrows the result and reports no difference,
// since an unknown inventory is not evidence of a differing one.
func keepShared[T any](base, other []T, key func(T) string) ([]T, bool) {
	if other == nil {
		return base, false
	}
	if base == nil {
		return other, false
	}
	seen := make(map[string]bool, len(other))
	for _, o := range other {
		seen[key(o)] = true
	}
	kept := make([]T, 0, len(base))
	for _, b := range base {
		if seen[key(b)] {
			kept = append(kept, b)
		}
	}
	return kept, len(kept) != len(base) || len(kept) != len(other)
}

// storageNames and bridgeNames project a node's typed inventory down to the
// plain names config.ValidatePlacementAgainstInventory compares against.
// bridgeNames (sanitized display names, which are still valid equality
// keys here) lives in node_placement.go.
func storageNames(storage []proxmoxStorage) []string {
	names := make([]string, len(storage))
	for i, s := range storage {
		names[i] = s.Name
	}
	return names
}

// fetchNodeDetails pulls storage/bridges, best-effort — endpoint errors
// are swallowed to nil slices rather than failing the whole discovery.
func fetchNodeDetails(ctx context.Context, client *proxmox.Client, nodeName string) proxmoxNodeInventory {
	node, err := client.Node(ctx, nodeName)
	if err != nil {
		return proxmoxNodeInventory{}
	}

	var storage []proxmoxStorage
	storageKnown := false
	if stores, err := node.Storages(ctx); err == nil {
		storageKnown = true
		storage = make([]proxmoxStorage, 0, len(stores))
		for _, s := range stores {
			if s.Enabled == 0 {
				continue
			}
			storage = append(storage, proxmoxStorage{
				Name:       s.Name,
				Content:    s.Content,
				TotalGB:    int(s.Total / (1024 * 1024 * 1024)), //nolint:gosec // G115: uint64→int is safe for GB-scale storage
				TotalKnown: true,
			})
		}
	}

	var bridges []proxmoxBridge
	bridgesKnown := false
	if nets, err := node.Networks(ctx, "bridge"); err == nil {
		bridgesKnown = true
		bridges = make([]proxmoxBridge, 0, len(nets))
		for _, n := range nets {
			bridges = append(bridges, proxmoxBridge{
				Name: n.Iface,
				CIDR: n.CIDR,
			})
		}
	}

	return proxmoxNodeInventory{
		Storage: storage, StorageKnown: storageKnown,
		Bridges: bridges, BridgesKnown: bridgesKnown,
	}
}

// classifyError turns a raw discovery failure into an actionable message.
// A TLS trust failure gets its own safe remedy per real cause — install the
// CA, connect using a name the certificate covers, or renew it — rather
// than one bucket that nudges the operator toward disabling verification;
// a TLS failure typed checks can't attribute to a specific cause (or any
// other untyped x509/tls error) gets an honest generic message instead of a
// guessed one.
func classifyError(err error) error {
	var unknownAuthority x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthority) {
		return fmt.Errorf("tls certificate not trusted — install the proxmox host's ca certificate in your system trust store: %w", err)
	}

	var hostnameErr x509.HostnameError
	if errors.As(err, &hostnameErr) {
		return fmt.Errorf("tls certificate name mismatch — connect using a name on the certificate, or reissue the certificate for %q: %w", hostnameErr.Host, err)
	}

	var certInvalid x509.CertificateInvalidError
	if errors.As(err, &certInvalid) {
		if certInvalid.Reason == x509.Expired {
			return fmt.Errorf("tls certificate expired — renew the proxmox host's certificate: %w", err)
		}
		return fmt.Errorf("tls certificate invalid — check the proxmox host's certificate: %w", err)
	}

	msg := err.Error()
	switch {
	case strings.Contains(msg, "x509:") || strings.Contains(msg, "tls:"):
		return fmt.Errorf("tls handshake failed — check the proxmox host's certificate and tls configuration: %w", err)
	case strings.Contains(msg, "connection refused"):
		return fmt.Errorf("connection refused — check that the proxmox host and port are correct")
	case strings.Contains(msg, "no such host"):
		return fmt.Errorf("host not found — check the proxmox host address")
	case strings.Contains(msg, "i/o timeout") || strings.Contains(msg, "context deadline"):
		return fmt.Errorf("connection timed out — check that the proxmox host is reachable")
	case strings.Contains(msg, "status 401") || strings.Contains(msg, "authentication failure"):
		return fmt.Errorf("authentication failed — check username and password")
	default:
		return fmt.Errorf("connection failed — check proxmox connectivity and credentials: %w", err)
	}
}
