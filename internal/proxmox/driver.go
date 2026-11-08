package proxmox

import (
	"context"
	"net/http"
	"time"

	"github.com/luthermonson/go-proxmox"
)

// VMRef addresses a VM by the node currently hosting it and its VMID.
type VMRef struct {
	Node string
	VMID int
}

// Task identifies a Proxmox task by its node and UPID.
type Task struct {
	Node string
	UPID string
}

// Version is the cluster-wide Proxmox VE release reported by GET /version.
type Version struct {
	Version string
	Release string
}

// VMStatus is a VM's power state as GET /nodes/{node}/qemu/{vmid}/status/current reports it.
type VMStatus struct {
	Status    string
	QMPStatus string
	Lock      string
	HA        bool
}

// VMSummary is one row of GET /cluster/resources?type=vm.
type VMSummary struct {
	VMRef
	Name      string
	Status    string
	Tags      []string
	Pool      string
	MaxMemMiB int
	MaxCPU    int
}

// DiskSpec describes one SCSI disk to allocate on Storage at the given Slot.
type DiskSpec struct {
	Slot    string
	Storage string
	SizeGiB int
	Serial  string
}

// VMSpec is the desired shape of a VM at create time.
type VMSpec struct {
	VMID       int
	Name       string
	CPUType    string
	Cores      int
	MemoryMiB  int
	Disks      []DiskSpec
	EFIStorage string
	MAC        string
	Bridge     string
	VLAN       int
	ISOVolID   string
	Tag        string
	Pool       string
}

// VMPatch holds the mutable VM fields; a nil pointer or nil slice leaves that field untouched.
type VMPatch struct {
	Cores     *int
	MemoryMiB *int
	Tags      []string
	OnBoot    *bool
	Delete    []string
}

// VMConfig is a VM's normalised configuration; a zero field means PVE has
// the key unset, and Digest is the optimistic-lock token UpdateVM requires.
type VMConfig struct {
	Name         string
	Cores        int
	Sockets      int
	CPUType      string
	MemoryMiB    int
	Balloon      int
	BIOS         string
	Machine      string
	SCSIHW       string
	OSType       string
	OnBoot       bool
	AgentEnabled bool
	BootOrder    []string
	Tags         []string
	NICModel     string
	MAC          string
	Bridge       string
	VLAN         int
	Disks        map[string]DiskConfig
	EFI          *EFIConfig
	CDROM        string
	Digest       string
}

// DiskConfig is one hard disk as the VM config reports it; VolID is the
// Proxmox-assigned volume, Storage the pool it lives on.
type DiskConfig struct {
	Storage  string
	VolID    string
	SizeGiB  int
	Serial   string
	IOThread bool
	Discard  bool
	Cache    string
	AIO      string
}

// EFIConfig is the OVMF variables disk.
type EFIConfig struct {
	Storage         string
	VolID           string
	EFIType         string
	PreEnrolledKeys bool
}

// Change is one difference between a VM's config and its spec; NeedsPowerCycle
// marks a key PVE queues as pending until the next cold boot.
type Change struct {
	Field           string
	Want            string
	Have            string
	NeedsPowerCycle bool
}

// Pending is one queued configuration change that applies on the next power cycle.
type Pending struct {
	Key     string
	Value   string
	Pending string
	Delete  bool
}

// NodeInfo is a cluster node's capacity and CPU capabilities.
type NodeInfo struct {
	Name        string
	Status      string
	MemTotalMiB int
	MemUsedMiB  int
	CPUs        int
	CPUModel    string
	CPUFlags    []string
	PVEVersion  string
}

// StorageInfo is a node's view of one storage pool.
type StorageInfo struct {
	Name       string
	Type       string
	Shared     bool
	Active     bool
	Content    []string
	AvailBytes int64
	TotalBytes int64
}

// Bridge is a Linux bridge on a node.
type Bridge struct {
	Name      string
	VLANAware bool
	Active    bool
}

// Volume is one stored volume, such as an ISO image.
type Volume struct {
	VolID string
	Size  int64
	CTime time.Time
}

// HASpec is the desired HA resource state and group for a VM.
type HASpec struct {
	State string
	Group string
}

// Permissions maps an ACL path to the privileges granted on it.
type Permissions map[string]map[string]bool

// Driver is the Proxmox VE API surface okdctl provisions through; every
// method returning a Task leaves the operation running until WaitTask
// confirms it.
type Driver interface {
	Version(ctx context.Context) (Version, error)
	Permissions(ctx context.Context) (Permissions, error)
	Nodes(ctx context.Context) ([]NodeInfo, error)
	Storages(ctx context.Context, node string) ([]StorageInfo, error)
	Bridges(ctx context.Context, node string) ([]Bridge, error)
	ListVMs(ctx context.Context) ([]VMSummary, error)
	VMConfig(ctx context.Context, ref VMRef) (VMConfig, error)
	VMPending(ctx context.Context, ref VMRef) ([]Pending, error)
	CreateVM(ctx context.Context, node string, spec VMSpec) (Task, error)
	UpdateVM(ctx context.Context, ref VMRef, patch VMPatch, digest string) (Task, error)
	ResizeDisk(ctx context.Context, ref VMRef, disk, size string) (Task, error)
	DetachISO(ctx context.Context, ref VMRef) (Task, error)
	DeleteVM(ctx context.Context, ref VMRef) (Task, error)
	Start(ctx context.Context, ref VMRef) (Task, error)
	Shutdown(ctx context.Context, ref VMRef, timeout time.Duration, force bool) (Task, error)
	Stop(ctx context.Context, ref VMRef) (Task, error)
	Reset(ctx context.Context, ref VMRef) (Task, error)
	Status(ctx context.Context, ref VMRef) (VMStatus, error)
	WaitTask(ctx context.Context, task Task) error
	UploadISO(ctx context.Context, node, storage, path, sha256 string) (Task, error)
	ListISOs(ctx context.Context, node, storage string) ([]Volume, error)
	DeleteVolume(ctx context.Context, node, storage, volid string) (Task, error)
	SetHA(ctx context.Context, ref VMRef, spec HASpec) error
	ClearHA(ctx context.Context, ref VMRef) error
}

// New returns a Driver for the Proxmox API at endpoint authenticated with an
// id=secret API token; token is caller-owned and may be zeroized after New returns.
func New(endpoint string, token []byte, insecure bool) (Driver, error) {
	// No transport timeout: ISO uploads outlive any fixed budget, so every
	// method is bounded by its ctx instead.
	client, httpClient, err := newProxmoxClient(endpoint, "", nil, token, insecure, 0)
	if err != nil {
		return nil, err
	}
	return &driver{
		client:       client,
		httpClient:   httpClient,
		baseURL:      APIBaseURL(endpoint),
		auth:         "PVEAPIToken=" + string(token),
		pollInterval: taskPollInterval,
	}, nil
}

// driver holds the go-proxmox client plus the raw-request fields UploadISO
// needs; auth is a private copy of the token header, so zeroizing the
// caller's token never touches it.
type driver struct {
	client       *proxmox.Client
	httpClient   *http.Client
	baseURL      string
	auth         string
	pollInterval time.Duration
}
