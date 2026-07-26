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

// proxmoxNode is a discovered Proxmox cluster node.
type proxmoxNode struct {
	Name   string
	Status string // "online" or "offline"
	CPUs   int
	MemGB  int
}

// proxmoxStorage is a discovered storage pool.
type proxmoxStorage struct {
	Name    string
	Type    string // lvm, lvmthin, dir, nfs, ceph, zfspool, etc.
	Content string // comma-separated: images, iso, backup, etc.
	Enabled bool
	TotalGB int
	UsedPct float64
}

// proxmoxBridge is a discovered network bridge.
type proxmoxBridge struct {
	Name   string
	Active bool
	CIDR   string // e.g. "192.168.1.1/24" if configured
}

// proxmoxDiscovery holds everything discovered from a Proxmox cluster.
type proxmoxDiscovery struct {
	Nodes   []proxmoxNode
	Storage []proxmoxStorage
	Bridges []proxmoxBridge
	ISOs    []string // storage volids of ISO files, e.g. "local:iso/fcos.iso"
}

// discoverProxmox queries the Proxmox API to list nodes, storage, and bridges.
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
		return nil, fmt.Errorf("no nodes found in cluster")
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

	targetNode := nodes[0].Name
	for _, n := range nodes {
		if n.Status == "online" {
			targetNode = n.Name
			break
		}
	}

	storage, bridges, isos := fetchNodeDetails(ctx, client, targetNode)

	return &proxmoxDiscovery{
		Nodes:   nodes,
		Storage: storage,
		Bridges: bridges,
		ISOs:    isos,
	}, nil
}

// fetchNodeDetails pulls storage, bridges, and ISO volids from the given
// node. Errors are swallowed to nil slices — discovery is best-effort, and
// a partial result beats no result when one endpoint misbehaves.
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
				Type:    s.Type,
				Content: s.Content,
				Enabled: s.Enabled == 1,
				TotalGB: int(s.Total / (1024 * 1024 * 1024)), //nolint:gosec // G115: uint64→int is safe for GB-scale storage
				UsedPct: s.UsedFraction,
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
				Name:   n.Iface,
				Active: n.Active == 1,
				CIDR:   n.CIDR,
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

// classifyError maps raw HTTP/TLS errors to user-friendly messages.
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
		return fmt.Errorf("connection failed: %w", err)
	}
}
