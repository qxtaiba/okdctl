package steps

import (
	"context"
	"errors"
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
}

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

	var onlineNames []string
	for _, n := range nodes {
		if n.Status == "online" {
			onlineNames = append(onlineNames, n.Name)
		}
	}
	if len(onlineNames) == 0 {
		onlineNames = []string{nodes[0].Name}
	}

	// Every online node is a placement candidate (node_placement.go offers
	// all of them for per-VM assignment), so storage/bridges/isos must be
	// fetched from ALL of them, not sampled from one — otherwise the wizard
	// can offer a pool/bridge that only exists on the node it happened to
	// sample, which a role assigned to a different node can't reach.
	var failures []error
	storageSets := make([][]proxmoxStorage, 0, len(onlineNames))
	bridgeSets := make([][]proxmoxBridge, 0, len(onlineNames))
	isoSets := make([][]string, 0, len(onlineNames))
	inventory := make(map[string]config.ProxmoxNodeInventory, len(onlineNames))
	for _, name := range onlineNames {
		storage, bridges, isos, detailErr := fetchNodeDetails(ctx, client, name)
		if detailErr != nil {
			failures = append(failures, detailErr)
		}
		storageSets = append(storageSets, storage)
		bridgeSets = append(bridgeSets, bridges)
		isoSets = append(isoSets, isos)
		inventory[name] = config.ProxmoxNodeInventory{
			Storage: storageNames(storage),
			Bridges: bridgeNames(bridges),
		}
	}

	commonStorage, storageDiffers := intersectByKey(storageSets, func(s proxmoxStorage) string { return s.Name })
	commonBridges, bridgesDiffer := intersectByKey(bridgeSets, func(b proxmoxBridge) string { return b.Name })
	commonISOs, isosDiffer := intersectByKey(isoSets, func(v string) string { return v })
	if storageDiffers || bridgesDiffer || isosDiffer {
		failures = append(failures, fmt.Errorf("proxmox nodes report different storage pools, bridges, or isos — offering only what all %d online node(s) share", len(onlineNames)))
	}

	if result := config.ValidatePlacementAgainstInventory(cfg, inventory); !result.IsValid() {
		failures = append(failures, errors.New(result.Error()))
	}

	return &proxmoxDiscovery{
		Nodes:   nodes,
		Storage: commonStorage,
		Bridges: commonBridges,
		ISOs:    commonISOs,
	}, errors.Join(failures...)
}

// intersectByKey returns, in sets[0]'s order, the deduplicated elements
// whose key appears in every set, plus whether any set's keys weren't
// shared by all the others.
func intersectByKey[T any](sets [][]T, key func(T) string) ([]T, bool) {
	if len(sets) == 0 {
		return nil, false
	}
	counts := make(map[string]int)
	for _, set := range sets {
		seen := make(map[string]bool, len(set))
		for _, v := range set {
			k := key(v)
			if seen[k] {
				continue
			}
			seen[k] = true
			counts[k]++
		}
	}
	var common []T
	seen := make(map[string]bool, len(sets[0]))
	for _, v := range sets[0] {
		k := key(v)
		if counts[k] == len(sets) && !seen[k] {
			seen[k] = true
			common = append(common, v)
		}
	}
	differ := false
	for _, c := range counts {
		if c != len(sets) {
			differ = true
			break
		}
	}
	return common, differ
}

func storageNames(storage []proxmoxStorage) []string {
	names := make([]string, len(storage))
	for i, s := range storage {
		names[i] = s.Name
	}
	return names
}

// fetchNodeDetails retains successful observations alongside endpoint failures.
func fetchNodeDetails(ctx context.Context, client *proxmox.Client, nodeName string) ([]proxmoxStorage, []proxmoxBridge, []string, error) {
	node, err := client.Node(ctx, nodeName)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("inspect proxmox node %s: %w", nodeName, err)
	}

	var failures []error
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
	} else {
		failures = append(failures, fmt.Errorf("list storage: %w", err))
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
	} else {
		failures = append(failures, fmt.Errorf("list bridges: %w", err))
	}

	var isos []string
	for _, storeName := range isoStorageNames {
		st, err := node.Storage(ctx, storeName)
		if err != nil {
			failures = append(failures, fmt.Errorf("inspect ISO storage %s: %w", storeName, err))
			continue
		}
		contents, err := st.GetContent(ctx)
		if err != nil {
			failures = append(failures, fmt.Errorf("list ISOs in %s: %w", storeName, err))
			continue
		}
		for _, c := range contents {
			if strings.HasSuffix(strings.ToLower(c.Volid), ".iso") {
				isos = append(isos, c.Volid)
			}
		}
	}

	return storage, bridges, isos, errors.Join(failures...)
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
		return fmt.Errorf("connection failed: %w", err)
	}
}
