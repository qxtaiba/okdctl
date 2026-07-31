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

// ProbeOptions carries the read-only Proxmox API probe inputs. Password /
// APIToken are the caller's credential bytes; the caller owns Zeroize. Node is
// the Proxmox node the cluster VMs run on; Datastores are the storage names to
// report headroom for (typically os_storage + data_storage).
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

// DatastoreInfo is one datastore's capacity, used to judge headroom for
// double-provisioning during a Ceph migration.
type DatastoreInfo struct {
	Name       string
	TotalBytes uint64
	AvailBytes uint64
}

// HostProbe is the read-only snapshot ProbeHost returns. GuestAllocatedBytes is
// the sum of configured memory across running guests on Node (the over-commit
// figure the memory-budget guard needs); HostMemUsedBytes is the node's live
// usage, reported for context but not used by the guard.
type HostProbe struct {
	Node                string
	HostMemTotalBytes   uint64
	HostMemUsedBytes    uint64
	GuestAllocatedBytes uint64
	Datastores          []DatastoreInfo
}

// HostMemTotalMiB reports physical host memory in MiB for the memory-budget guard.
func (h *HostProbe) HostMemTotalMiB() int { return bytesToMiB(h.HostMemTotalBytes) }

// GuestAllocatedMiB reports summed running-guest memory in MiB for the guard.
func (h *HostProbe) GuestAllocatedMiB() int { return bytesToMiB(h.GuestAllocatedBytes) }

func bytesToMiB(b uint64) int { return int(b / (1024 * 1024)) } //nolint:gosec // G115: MiB-scale value fits int

// ProbeHost reads host memory, guest memory allocation, and datastore capacity
// from the Proxmox API. It performs ONLY reads — no VM mutation — so it does not
// violate the "all Proxmox mutations flow through terraform" invariant. It runs
// from the bastion over HTTPS (the SSH/pvesh path is denied there), reusing the
// go-proxmox wiring the wizard discovery uses.
func ProbeHost(ctx context.Context, opts *ProbeOptions) (*HostProbe, error) {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultProbeTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client, err := newProbeClient(opts, timeout)
	if err != nil {
		return nil, err
	}

	nodes, err := client.Nodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list proxmox nodes: %w", err)
	}
	total, used, err := selectNodeMem(nodes, opts.Node)
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
		HostMemUsedBytes:    used,
		GuestAllocatedBytes: sumRunningGuestMem(resources, opts.Node),
	}

	// Per-datastore reads are best-effort: one unreadable store must not sink
	// the whole probe, so a failing Storage lookup is skipped, not fatal.
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

// VMPowerStates reads the power state of the given QEMU vmids from the
// cluster resources listing — one read call over the same client path
// ProbeHost uses. VMs absent from the listing (destroyed out of band) are
// omitted from the result. opts.Node is not required: the listing is
// cluster-wide.
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

// mapVMStates folds the qemu entries matching vmids onto the VMState wire
// vocabulary; status strings outside it map to StateUnknown.
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

func newProbeClient(opts *ProbeOptions, timeout time.Duration) (*proxmox.Client, error) {
	if opts.Node == "" {
		return nil, fmt.Errorf("proxmox probe: node is required")
	}
	return newProxmoxClient(opts.Endpoint, opts.Username, opts.Password, opts.APIToken, opts.Insecure, timeout)
}

// newProxmoxClient builds a read/operational go-proxmox client shared by the
// host probe and the power-cycler. Token auth is the headless bastion
// default. go-proxmox needs the token split into id=secret; the credential
// []byte becomes an immutable Go string inside the client that Zeroize cannot
// reach — bounded to the single call, the caller still wipes its own copy.
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

// warnInsecureTLS surfaces a long-forgotten insecure:true once per process —
// PowerCycler builds a client per operation, so per-call warnings would be
// noise. It logs via slog.Default, which the cli rebinds to the
// RedactHandler-backed logger; no logger threads through the credential
// options structs, and widening them for one warning is not worth it.
func warnInsecureTLS(endpoint string) {
	insecureTLSWarnOnce.Do(func() {
		slog.Warn("proxmox tls verification disabled (insecure: true)", "endpoint", endpoint)
	})
}

// selectNodeMem returns the physical and live-used memory of the named node.
func selectNodeMem(nodes proxmox.NodeStatuses, name string) (total, used uint64, err error) {
	for _, n := range nodes {
		if n.Node == name {
			return n.MaxMem, n.Mem, nil
		}
	}
	return 0, 0, fmt.Errorf("proxmox probe: node %q not found in cluster", name)
}

// sumRunningGuestMem sums configured memory (MaxMem) across running qemu guests
// on node. Configured memory, not live usage, is the over-commit figure: a VM
// can touch its full ceiling at any time, so the budget must plan for it.
func sumRunningGuestMem(resources proxmox.ClusterResources, node string) uint64 {
	var total uint64
	for _, r := range resources {
		if r.Type == "qemu" && r.Node == node && r.Status == "running" {
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

// APIBaseURL normalizes endpoint (https scheme by default, trailing slashes
// trimmed) and appends the /api2/json API root. It is the single place the
// suffix is written; every go-proxmox client (probe, power-cycler, wizard
// discovery) builds its base URL here.
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
