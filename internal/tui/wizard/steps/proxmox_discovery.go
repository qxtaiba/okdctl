package steps

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
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
	Nodes    []proxmoxNode
	Storage  []proxmoxStorage
	Bridges  []proxmoxBridge
}

// discoverProxmox queries the Proxmox API to list nodes, storage, and bridges.
func discoverProxmox(cfg *config.Config) (*proxmoxDiscovery, error) {
	if cfg.Provider.Proxmox == nil {
		return nil, fmt.Errorf("no proxmox config")
	}

	px := cfg.Provider.Proxmox
	if px.Host == "" || px.Username == "" || px.Password == "" {
		return nil, fmt.Errorf("missing credentials — enter host, username, and password in the proxmox step")
	}

	baseURL := buildBaseURL(px.Host)
	client := buildHTTPClient(px.Insecure)

	ticket, err := getAuthTicket(client, baseURL, px.Username, px.Password)
	if err != nil {
		return nil, classifyError(err)
	}

	nodes, err := fetchNodes(client, baseURL, ticket)
	if err != nil {
		return nil, fmt.Errorf("fetch nodes: %w", err)
	}

	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes found in cluster")
	}

	// Fetch storage and bridges from the first online node
	targetNode := nodes[0].Name
	for _, n := range nodes {
		if n.Status == "online" {
			targetNode = n.Name
			break
		}
	}

	storage, _ := fetchStorage(client, baseURL, ticket, targetNode)
	bridges, _ := fetchBridges(client, baseURL, ticket, targetNode)

	return &proxmoxDiscovery{
		Nodes:   nodes,
		Storage: storage,
		Bridges: bridges,
	}, nil
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

func buildBaseURL(host string) string {
	if strings.Contains(host, "://") {
		return strings.TrimRight(host, "/")
	}
	return "https://" + strings.TrimRight(host, "/")
}

func buildHTTPClient(insecure bool) *http.Client {
	transport := &http.Transport{}
	if insecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // user opted in
	}
	return &http.Client{
		Timeout:   10 * time.Second,
		Transport: transport,
	}
}

// --- Auth ---

type ticketResponse struct {
	Data struct {
		Ticket string `json:"ticket"`
	} `json:"data"`
}

func getAuthTicket(client *http.Client, baseURL, username, password string) (string, error) {
	resp, err := client.PostForm(baseURL+"/api2/json/access/ticket", url.Values{
		"username": {username},
		"password": {password},
	})
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var result ticketResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode ticket: %w", err)
	}
	if result.Data.Ticket == "" {
		return "", fmt.Errorf("empty ticket in response")
	}

	return result.Data.Ticket, nil
}

// apiGet performs an authenticated GET request using a PVE ticket cookie.
// GET requests do not require a CSRFPreventionToken — only POST/PUT/DELETE do.
func apiGet(client *http.Client, url, ticket string, target any) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.AddCookie(&http.Cookie{Name: "PVEAuthCookie", Value: ticket})

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(target)
}

// --- Nodes ---

func fetchNodes(client *http.Client, baseURL, ticket string) ([]proxmoxNode, error) {
	var result struct {
		Data []struct {
			Node   string  `json:"node"`
			Status string  `json:"status"`
			MaxCPU int     `json:"maxcpu"`
			MaxMem float64 `json:"maxmem"`
		} `json:"data"`
	}

	if err := apiGet(client, baseURL+"/api2/json/nodes", ticket, &result); err != nil {
		return nil, err
	}

	nodes := make([]proxmoxNode, 0, len(result.Data))
	for _, n := range result.Data {
		nodes = append(nodes, proxmoxNode{
			Name:   n.Node,
			Status: n.Status,
			CPUs:   n.MaxCPU,
			MemGB:  int(n.MaxMem / (1024 * 1024 * 1024)),
		})
	}
	return nodes, nil
}

// --- Storage ---

func fetchStorage(client *http.Client, baseURL, ticket, node string) ([]proxmoxStorage, error) {
	var result struct {
		Data []struct {
			Storage      string  `json:"storage"`
			Type         string  `json:"type"`
			Content      string  `json:"content"`
			Enabled      int     `json:"enabled"`
			Active       int     `json:"active"`
			Total        float64 `json:"total"`
			UsedFraction float64 `json:"used_fraction"`
		} `json:"data"`
	}

	if err := apiGet(client, baseURL+"/api2/json/nodes/"+node+"/storage", ticket, &result); err != nil {
		return nil, err
	}

	var pools []proxmoxStorage
	for _, s := range result.Data {
		if s.Enabled == 0 {
			continue
		}
		pools = append(pools, proxmoxStorage{
			Name:    s.Storage,
			Type:    s.Type,
			Content: s.Content,
			Enabled: s.Enabled == 1,
			TotalGB: int(s.Total / (1024 * 1024 * 1024)),
			UsedPct: s.UsedFraction,
		})
	}
	return pools, nil
}

// --- Bridges ---

func fetchBridges(client *http.Client, baseURL, ticket, node string) ([]proxmoxBridge, error) {
	var result struct {
		Data []struct {
			Iface  string `json:"iface"`
			Type   string `json:"type"`
			Active int    `json:"active"`
			CIDR   string `json:"cidr"`
		} `json:"data"`
	}

	if err := apiGet(client, baseURL+"/api2/json/nodes/"+node+"/network", ticket, &result); err != nil {
		return nil, err
	}

	var bridges []proxmoxBridge
	for _, iface := range result.Data {
		if iface.Type != "bridge" {
			continue
		}
		bridges = append(bridges, proxmoxBridge{
			Name:   iface.Iface,
			Active: iface.Active == 1,
			CIDR:   iface.CIDR,
		})
	}
	return bridges, nil
}
