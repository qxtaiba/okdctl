package proxmox

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/luthermonson/go-proxmox"

	"github.com/qxtaiba/okdctl/internal/httputil"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

// defaultProbeTimeout bounds every read in a single ProbeHost call.
const defaultProbeTimeout = 15 * time.Second

// ProbeOptions carries the read-only Proxmox probe inputs; Password/APIToken
// are caller-owned bytes the caller must Zeroize.
type ProbeOptions struct {
	Endpoint   string
	Username   string
	Password   []byte
	APIToken   []byte
	Insecure   bool
	Node       string
	Datastores []string
	Timeout    time.Duration
}

// redactedProbeOptions is ProbeOptions without Password/APIToken, safe to
// format (slog's redactAny).
type redactedProbeOptions struct {
	Endpoint   string
	Username   string
	Insecure   bool
	Node       string
	Datastores []string
	Timeout    time.Duration
}

// Redacted returns o's non-secret fields, for the logutil.redactAny interface.
func (o *ProbeOptions) Redacted() any {
	if o == nil {
		return nil
	}
	return redactedProbeOptions{
		Endpoint:   o.Endpoint,
		Username:   o.Username,
		Insecure:   o.Insecure,
		Node:       o.Node,
		Datastores: o.Datastores,
		Timeout:    o.Timeout,
	}
}

// String masks secret fields so accidental %v/%s/log calls can't leak them.
func (o *ProbeOptions) String() string {
	if o == nil {
		return "ProbeOptions(nil)"
	}
	return fmt.Sprintf("%+v", o.Redacted())
}

// DatastoreInfo is a datastore's capacity, used to judge Ceph-migration headroom.
type DatastoreInfo struct {
	Name       string
	TotalBytes uint64
	AvailBytes uint64
}

// HostProbe is the read-only snapshot ProbeHost returns; GuestAllocatedBytes
// sums running-guest memory on Node for the memory-budget over-commit guard.
type HostProbe struct {
	Node                string
	HostMemTotalBytes   uint64
	GuestAllocatedBytes uint64
	Datastores          []DatastoreInfo
}

// HostMemTotalMiB reports physical host memory in MiB for the memory-budget guard.
func (h *HostProbe) HostMemTotalMiB() int { return bytesToMiB(h.HostMemTotalBytes) }

// GuestAllocatedMiB reports summed running-guest memory in MiB for the guard.
func (h *HostProbe) GuestAllocatedMiB() int { return bytesToMiB(h.GuestAllocatedBytes) }

func bytesToMiB(b uint64) int { return int(b / (1024 * 1024)) } //nolint:gosec // G115: MiB-scale value fits int

// ProbeHost reads host memory, guest allocation, and datastore capacity via
// the Proxmox API — read-only (exempt from terraform-only mutations) over
// HTTPS since the bastion has no SSH path to Proxmox.
func ProbeHost(ctx context.Context, opts *ProbeOptions) (*HostProbe, error) {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultProbeTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if opts.Node == "" {
		return nil, fmt.Errorf("proxmox probe: node is required")
	}
	client, err := newProxmoxClient(opts.Endpoint, opts.Username, opts.Password, opts.APIToken, opts.Insecure, timeout)
	if err != nil {
		return nil, err
	}

	nodes, err := client.Nodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list proxmox nodes: %w", err)
	}
	total, err := selectNodeMem(nodes, opts.Node)
	if err != nil {
		return nil, err
	}

	cluster, err := client.Cluster(ctx)
	if err != nil {
		return nil, fmt.Errorf("get proxmox cluster: %w", err)
	}
	resources, err := cluster.Resources(ctx, "vm")
	if err != nil {
		return nil, fmt.Errorf("list cluster vm resources: %w", err)
	}

	probe := &HostProbe{
		Node:                opts.Node,
		HostMemTotalBytes:   total,
		GuestAllocatedBytes: sumRunningGuestMem(resources, opts.Node),
	}

	// Per-datastore reads are best-effort: a failing lookup is skipped, not fatal.
	node, err := client.Node(ctx, opts.Node)
	if err == nil {
		for _, name := range dedupe(opts.Datastores) {
			st, stErr := node.Storage(ctx, name)
			if stErr != nil {
				continue
			}
			probe.Datastores = append(probe.Datastores, DatastoreInfo{
				Name:       name,
				TotalBytes: st.Total,
				AvailBytes: st.Avail,
			})
		}
	}

	return probe, nil
}

// VMPowerStates reads vmids' power state from the cluster-wide resources
// listing (opts.Node is not required); absent VMs are omitted, not defaulted.
func VMPowerStates(ctx context.Context, opts *ProbeOptions, vmids []int) (map[int]nodetypes.VMState, error) {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultProbeTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client, err := newProxmoxClient(opts.Endpoint, opts.Username, opts.Password, opts.APIToken, opts.Insecure, timeout)
	if err != nil {
		return nil, err
	}
	cluster, err := client.Cluster(ctx)
	if err != nil {
		return nil, fmt.Errorf("get proxmox cluster: %w", err)
	}
	resources, err := cluster.Resources(ctx, "vm")
	if err != nil {
		return nil, fmt.Errorf("list cluster vm resources: %w", err)
	}
	return mapVMStates(resources, vmids), nil
}

// mapVMStates maps qemu entries in vmids to the VMState vocabulary;
// unrecognized statuses become StateUnknown.
func mapVMStates(resources proxmox.ClusterResources, vmids []int) map[int]nodetypes.VMState {
	want := make(map[uint64]bool, len(vmids))
	for _, id := range vmids {
		if id > 0 {
			want[uint64(id)] = true
		}
	}
	states := make(map[int]nodetypes.VMState, len(vmids))
	for _, r := range resources {
		if r.Type != "qemu" || !want[r.VMID] {
			continue
		}
		state := nodetypes.VMState(r.Status)
		if state != nodetypes.StateRunning && state != nodetypes.StateStopped {
			state = nodetypes.StateUnknown
		}
		states[int(r.VMID)] = state //nolint:gosec // G115: qemu vmids are small positive integers
	}
	return states
}

// newProxmoxClient builds the shared go-proxmox client; the credential
// becomes an immutable Go string Zeroize can't reach, bounded to this call.
func newProxmoxClient(endpoint, username string, password, apiToken []byte, insecure bool, timeout time.Duration) (*proxmox.Client, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("proxmox client: endpoint is required")
	}
	if insecure {
		warnInsecureTLS(endpoint)
	}
	httpClient := httputil.NewOptionalInsecure(insecure, timeout)
	base := APIBaseURL(endpoint)

	switch {
	case len(password) > 0:
		return proxmox.NewClient(
			base,
			proxmox.WithHTTPClient(httpClient),
			proxmox.WithCredentials(&proxmox.Credentials{Username: username, Password: string(password)}),
		), nil
	case len(apiToken) > 0:
		id, secret, ok := strings.Cut(string(apiToken), "=")
		if !ok {
			return nil, fmt.Errorf("proxmox client: PROXMOX_VE_API_TOKEN must be in id=secret form")
		}
		return proxmox.NewClient(
			base,
			proxmox.WithHTTPClient(httpClient),
			proxmox.WithAPIToken(id, secret),
		), nil
	default:
		return nil, fmt.Errorf("proxmox client: no credentials (need password or api token)")
	}
}

var insecureTLSWarnOnce sync.Once

// warnInsecureTLS logs insecure:true once per process (PowerCycler builds a
// client per call) via slog.Default rather than threading a logger through.
func warnInsecureTLS(endpoint string) {
	insecureTLSWarnOnce.Do(func() {
		slog.Warn("proxmox tls verification disabled (insecure: true)", "endpoint", endpoint)
	})
}

func selectNodeMem(nodes proxmox.NodeStatuses, name string) (uint64, error) {
	for _, n := range nodes {
		if n.Node == name {
			return n.MaxMem, nil
		}
	}
	return 0, fmt.Errorf("proxmox probe: node %q not found in cluster", name)
}

// sumRunningGuestMem sums configured memory (not live usage) across running
// qemu guests on node — the over-commit figure a VM could touch at any time.
func sumRunningGuestMem(resources proxmox.ClusterResources, node string) uint64 {
	var total uint64
	for _, r := range resources {
		if r.Type == "qemu" && r.Node == node && r.Status == string(nodetypes.StateRunning) {
			total += r.MaxMem
		}
	}
	return total
}

func dedupe(names []string) []string {
	seen := make(map[string]bool, len(names))
	var out []string
	for _, n := range names {
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

// APIBaseURL normalizes endpoint (https default, trailing slash trimmed) and
// appends /api2/json; every go-proxmox client builds its base URL here.
func APIBaseURL(endpoint string) string {
	return normalizeEndpoint(endpoint) + "/api2/json"
}

func normalizeEndpoint(endpoint string) string {
	e := strings.TrimRight(endpoint, "/")
	if strings.Contains(e, "://") {
		return e
	}
	return "https://" + e
}
