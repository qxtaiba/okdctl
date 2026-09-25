package steps

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/luthermonson/go-proxmox"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/httputil"
	infraproxmox "github.com/qxtaiba/okdctl/internal/infrastructure/proxmox"
)

type proxmoxNode struct {
	Name   string
	Status string // "online" or "offline"
	CPUs   int
	MemGB  int
}

type proxmoxStorage struct {
	Name    string
	Content string // comma-separated: images, iso, backup, etc.
	TotalGB int
}

type proxmoxBridge struct {
	Name string
	CIDR string // e.g. "192.168.1.1/24" if configured
}

type proxmoxDiscovery struct {
	Nodes   []proxmoxNode
	Storage []proxmoxStorage
	Bridges []proxmoxBridge
	ISOs    []string // storage volids of ISO files, e.g. "local:iso/fcos.iso"

	// Heterogeneous reports that online nodes' inventories differed, so
	// Storage/Bridges/ISOs hold only what every online node shares and the
	// placement step should say so.
	Heterogeneous bool
}

func discoverProxmox(cfg *config.Config) (*proxmoxDiscovery, error) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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
			Name:   n.Node,
			Status: n.Status,
			CPUs:   n.MaxCPU,
			MemGB:  int(n.MaxMem / (1024 * 1024 * 1024)), //nolint:gosec // G115: uint64→int is safe for GB-scale memory
		})
	}

	online := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if n.Status == "online" {
			online = append(online, n.Name)
		}
	}
	if len(online) == 0 {
		online = []string{nodes[0].Name}
	}

	storage, bridges, isos, heterogeneous := fetchClusterDetails(ctx, client, online)

	return &proxmoxDiscovery{
		Nodes:         nodes,
		Storage:       storage,
		Bridges:       bridges,
		ISOs:          isos,
		Heterogeneous: heterogeneous,
	}, nil
}

// fetchClusterDetails pulls storage/bridges/ISOs from every online node and
// keeps only what all of them share (by name), so the wizard's single
// cluster-wide pick lists never offer a resource missing on the node a VM
// lands on; heterogeneous reports whether any two inventories differed. A
// category whose fetch failed on a node (nil, best-effort) neither narrows
// the result nor counts as a difference.
func fetchClusterDetails(ctx context.Context, client *proxmox.Client, nodeNames []string) ([]proxmoxStorage, []proxmoxBridge, []string, bool) {
	storage, bridges, isos := fetchNodeDetails(ctx, client, nodeNames[0])
	heterogeneous := false

	for _, nodeName := range nodeNames[1:] {
		s, b, i := fetchNodeDetails(ctx, client, nodeName)
		var differs bool
		storage, differs = keepShared(storage, s, func(v proxmoxStorage) string { return v.Name })
		heterogeneous = heterogeneous || differs
		bridges, differs = keepShared(bridges, b, func(v proxmoxBridge) string { return v.Name })
		heterogeneous = heterogeneous || differs
		isos, differs = keepShared(isos, i, func(v string) string { return v })
		heterogeneous = heterogeneous || differs
	}

	return storage, bridges, isos, heterogeneous
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

// fetchNodeDetails pulls storage/bridges/ISOs, best-effort — endpoint errors
// are swallowed to nil slices rather than failing the whole discovery.
func fetchNodeDetails(ctx context.Context, client *proxmox.Client, nodeName string) ([]proxmoxStorage, []proxmoxBridge, []string) {
	node, err := client.Node(ctx, nodeName)
	if err != nil {
		return nil, nil, nil
	}

	var storage []proxmoxStorage
	var isoStorageNames []string
	if stores, err := node.Storages(ctx); err == nil {
		storage = make([]proxmoxStorage, 0, len(stores))
		for _, s := range stores {
			if s.Enabled == 0 {
				continue
			}
			storage = append(storage, proxmoxStorage{
				Name:    s.Name,
				Content: s.Content,
				TotalGB: int(s.Total / (1024 * 1024 * 1024)), //nolint:gosec // G115: uint64→int is safe for GB-scale storage
			})
			if strings.Contains(s.Content, "iso") {
				isoStorageNames = append(isoStorageNames, s.Name)
			}
		}
	}

	var bridges []proxmoxBridge
	if nets, err := node.Networks(ctx, "bridge"); err == nil {
		bridges = make([]proxmoxBridge, 0, len(nets))
		for _, n := range nets {
			bridges = append(bridges, proxmoxBridge{
				Name: n.Iface,
				CIDR: n.CIDR,
			})
		}
	}

	var isos []string
	for _, storeName := range isoStorageNames {
		st, err := node.Storage(ctx, storeName)
		if err != nil {
			continue
		}
		contents, err := st.GetContent(ctx)
		if err != nil {
			continue
		}
		for _, c := range contents {
			if strings.HasSuffix(strings.ToLower(c.Volid), ".iso") {
				isos = append(isos, c.Volid)
			}
		}
	}

	return storage, bridges, isos
}

func classifyError(err error) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "x509:") || strings.Contains(msg, "tls:"):
		return fmt.Errorf("tls certificate verification failed — go back and set \"skip tls verify\" to yes")
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
