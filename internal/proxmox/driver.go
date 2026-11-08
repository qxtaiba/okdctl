package proxmox

import (
	"context"
	"fmt"
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
}

// VMConfig is a VM's normalised configuration; Digest is the optimistic-lock
// token UpdateVM requires.
type VMConfig struct {
	Digest string
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
	client, err := newProxmoxClient(endpoint, "", nil, token, insecure, 0)
	if err != nil {
		return nil, err
	}
	return &driver{client: client}, nil
}

type driver struct {
	client *proxmox.Client
}

func notImplemented(method string) error {
	return fmt.Errorf("proxmox driver: %s not implemented", method)
}

func (d *driver) Version(context.Context) (Version, error) {
	return Version{}, notImplemented("Version")
}

func (d *driver) Permissions(context.Context) (Permissions, error) {
	return nil, notImplemented("Permissions")
}

func (d *driver) Nodes(context.Context) ([]NodeInfo, error) {
	return nil, notImplemented("Nodes")
}

func (d *driver) Storages(context.Context, string) ([]StorageInfo, error) {
	return nil, notImplemented("Storages")
}

func (d *driver) Bridges(context.Context, string) ([]Bridge, error) {
	return nil, notImplemented("Bridges")
}

func (d *driver) ListVMs(context.Context) ([]VMSummary, error) {
	return nil, notImplemented("ListVMs")
}

func (d *driver) VMConfig(context.Context, VMRef) (VMConfig, error) {
	return VMConfig{}, notImplemented("VMConfig")
}

func (d *driver) VMPending(context.Context, VMRef) ([]Pending, error) {
	return nil, notImplemented("VMPending")
}

func (d *driver) CreateVM(context.Context, string, VMSpec) (Task, error) {
	return Task{}, notImplemented("CreateVM")
}

func (d *driver) UpdateVM(context.Context, VMRef, VMPatch, string) (Task, error) {
	return Task{}, notImplemented("UpdateVM")
}

func (d *driver) ResizeDisk(context.Context, VMRef, string, string) (Task, error) {
	return Task{}, notImplemented("ResizeDisk")
}

func (d *driver) DetachISO(context.Context, VMRef) (Task, error) {
	return Task{}, notImplemented("DetachISO")
}

func (d *driver) DeleteVM(context.Context, VMRef) (Task, error) {
	return Task{}, notImplemented("DeleteVM")
}

func (d *driver) Start(context.Context, VMRef) (Task, error) {
	return Task{}, notImplemented("Start")
}

func (d *driver) Shutdown(context.Context, VMRef, time.Duration, bool) (Task, error) {
	return Task{}, notImplemented("Shutdown")
}

func (d *driver) Stop(context.Context, VMRef) (Task, error) {
	return Task{}, notImplemented("Stop")
}

func (d *driver) Reset(context.Context, VMRef) (Task, error) {
	return Task{}, notImplemented("Reset")
}

func (d *driver) Status(context.Context, VMRef) (VMStatus, error) {
	return VMStatus{}, notImplemented("Status")
}

func (d *driver) WaitTask(context.Context, Task) error {
	return notImplemented("WaitTask")
}

func (d *driver) UploadISO(context.Context, string, string, string, string) (Task, error) {
	return Task{}, notImplemented("UploadISO")
}

func (d *driver) ListISOs(context.Context, string, string) ([]Volume, error) {
	return nil, notImplemented("ListISOs")
}

func (d *driver) DeleteVolume(context.Context, string, string, string) (Task, error) {
	return Task{}, notImplemented("DeleteVolume")
}

func (d *driver) SetHA(context.Context, VMRef, HASpec) error {
	return notImplemented("SetHA")
}

func (d *driver) ClearHA(context.Context, VMRef) error {
	return notImplemented("ClearHA")
}
