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
	MemGB  int // total memory in GB
}

// discoverProxmoxNodes queries the Proxmox API to list all cluster nodes.
// Returns nil, err on failure (caller should fall back to manual input).
func discoverProxmoxNodes(cfg *config.Config) ([]proxmoxNode, error) {
	if cfg.Provider.Proxmox == nil {
		return nil, fmt.Errorf("no proxmox config")
	}

	px := cfg.Provider.Proxmox
	if px.Host == "" || px.Username == "" || px.Password == "" {
		return nil, fmt.Errorf("proxmox host, username, and password required for node discovery")
	}

	baseURL := buildBaseURL(px.Host)
	client := buildHTTPClient(px.Insecure)

	// Step 1: authenticate
	ticket, err := getAuthTicket(client, baseURL, px.Username, px.Password)
	if err != nil {
		return nil, fmt.Errorf("auth failed: %w", err)
	}

	// Step 2: fetch nodes
	nodes, err := fetchNodes(client, baseURL, ticket)
	if err != nil {
		return nil, fmt.Errorf("fetch nodes failed: %w", err)
	}

	return nodes, nil
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

type ticketResponse struct {
	Data struct {
		Ticket string `json:"ticket"`
	} `json:"data"`
}

func getAuthTicket(client *http.Client, baseURL, username, password string) (string, error) {
	form := url.Values{
		"username": {username},
		"password": {password},
	}

	resp, err := client.PostForm(baseURL+"/api2/json/access/ticket", form)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

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

type nodesResponse struct {
	Data []struct {
		Node   string  `json:"node"`
		Status string  `json:"status"`
		MaxCPU int     `json:"maxcpu"`
		MaxMem float64 `json:"maxmem"` // bytes, can be large
	} `json:"data"`
}

func fetchNodes(client *http.Client, baseURL, ticket string) ([]proxmoxNode, error) {
	req, err := http.NewRequest("GET", baseURL+"/api2/json/nodes", nil)
	if err != nil {
		return nil, err
	}
	req.AddCookie(&http.Cookie{Name: "PVEAuthCookie", Value: ticket})

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var result nodesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode nodes: %w", err)
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
