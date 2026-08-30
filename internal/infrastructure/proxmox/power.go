package proxmox

import (
	"context"
	"fmt"
	"time"

	"github.com/luthermonson/go-proxmox"
)

// defaultPowerCycleTimeout bounds each stop/start task within a power-cycle.
const defaultPowerCycleTimeout = 5 * time.Minute

const powerTaskPollInterval = 2 * time.Second

// PowerCycleOptions carries the Proxmox API credentials for a power-cycle;
// Password/APIToken are caller-owned bytes the caller must Zeroize.
type PowerCycleOptions struct {
	Endpoint string
	Username string
	Password []byte
	APIToken []byte
	Insecure bool
	Timeout  time.Duration
}

// redactedPowerCycleOptions is PowerCycleOptions without Password/APIToken,
// safe to format (slog's redactAny).
type redactedPowerCycleOptions struct {
	Endpoint string
	Username string
	Insecure bool
	Timeout  time.Duration
}

// Redacted returns o's non-secret fields, for the logutil.redactAny interface.
func (o *PowerCycleOptions) Redacted() any {
	if o == nil {
		return nil
	}
	return redactedPowerCycleOptions{
		Endpoint: o.Endpoint,
		Username: o.Username,
		Insecure: o.Insecure,
		Timeout:  o.Timeout,
	}
}

// String masks secret fields so accidental %v/%s/log calls can't leak them.
func (o *PowerCycleOptions) String() string {
	if o == nil {
		return "PowerCycleOptions(nil)"
	}
	return fmt.Sprintf("%+v", o.Redacted())
}

// PowerCycler stops then starts a VM via the Proxmox API: bpg/proxmox can't
// restart on resize (only stop→start picks up new RAM). Sanctioned exception
// to routing mutations through terraform, which can't express per-VM power.
type PowerCycler struct {
	opts *PowerCycleOptions
}

// NewPowerCycler returns a PowerCycler bound to the given API credentials.
func NewPowerCycler(opts *PowerCycleOptions) *PowerCycler {
	return &PowerCycler{opts: opts}
}

func (pc *PowerCycler) timeout() time.Duration {
	if pc.opts.Timeout > 0 {
		return pc.opts.Timeout
	}
	return defaultPowerCycleTimeout
}

// vm builds a timeout-scoped client; go-proxmox populates status/config on lookup.
func (pc *PowerCycler) vm(ctx context.Context, node string, vmid int, timeout time.Duration) (*proxmox.VirtualMachine, error) {
	client, err := newProxmoxClient(pc.opts.Endpoint, pc.opts.Username, pc.opts.Password, pc.opts.APIToken, pc.opts.Insecure, timeout)
	if err != nil {
		return nil, err
	}

	n, err := client.Node(ctx, node)
	if err != nil {
		return nil, fmt.Errorf("get proxmox node %s: %w", node, err)
	}
	vm, err := n.VirtualMachine(ctx, vmid)
	if err != nil {
		return nil, fmt.Errorf("get vm %d: %w", vmid, err)
	}
	return vm, nil
}

// PowerCycleVM stops (if running) then starts the VM, waiting for each task;
// it is fail-closed, leaving the caller to treat the resize as unrealized on error.
func (pc *PowerCycler) PowerCycleVM(ctx context.Context, node string, vmid int) error {
	timeout := pc.timeout()

	vm, err := pc.vm(ctx, node, vmid, timeout)
	if err != nil {
		return err
	}

	if vm.IsRunning() {
		stop, err := vm.Stop(ctx)
		if err != nil {
			return fmt.Errorf("stop vm %d: %w", vmid, err)
		}
		if err := stop.Wait(ctx, powerTaskPollInterval, timeout); err != nil {
			return fmt.Errorf("wait for vm %d stop: %w", vmid, err)
		}
	}

	start, err := vm.Start(ctx)
	if err != nil {
		return fmt.Errorf("start vm %d: %w", vmid, err)
	}
	if err := start.Wait(ctx, powerTaskPollInterval, timeout); err != nil {
		return fmt.Errorf("wait for vm %d start: %w", vmid, err)
	}
	return nil
}

// ShutdownVM sends ACPI shutdown then confirms via ping, since a completed
// task alone doesn't prove the guest powered off. No-op if already stopped.
func (pc *PowerCycler) ShutdownVM(ctx context.Context, node string, vmid int) error {
	timeout := pc.timeout()

	vm, err := pc.vm(ctx, node, vmid, timeout)
	if err != nil {
		return err
	}
	if vm.IsStopped() {
		return nil
	}

	task, err := vm.Shutdown(ctx)
	if err != nil {
		return fmt.Errorf("shutdown vm %d: %w", vmid, err)
	}
	if err := task.Wait(ctx, powerTaskPollInterval, timeout); err != nil {
		return fmt.Errorf("wait for vm %d shutdown: %w", vmid, err)
	}

	if err := vm.Ping(ctx); err != nil {
		return fmt.Errorf("confirm vm %d shutdown: %w", vmid, err)
	}
	if !vm.IsStopped() {
		return fmt.Errorf("confirm vm %d shutdown: vm still running after shutdown task completed", vmid)
	}
	return nil
}

// StartVM starts the VM and waits for the task; a no-op if already running.
func (pc *PowerCycler) StartVM(ctx context.Context, node string, vmid int) error {
	timeout := pc.timeout()

	vm, err := pc.vm(ctx, node, vmid, timeout)
	if err != nil {
		return err
	}
	if vm.IsRunning() {
		return nil
	}

	task, err := vm.Start(ctx)
	if err != nil {
		return fmt.Errorf("start vm %d: %w", vmid, err)
	}
	if err := task.Wait(ctx, powerTaskPollInterval, timeout); err != nil {
		return fmt.Errorf("wait for vm %d start: %w", vmid, err)
	}
	return nil
}
