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

// PowerCycleOptions carries the Proxmox API credentials for a power-cycle.
// Password/APIToken are the caller's credential bytes; the caller owns Zeroize.
type PowerCycleOptions struct {
	Endpoint string
	Username string
	Password []byte
	APIToken []byte
	Insecure bool
	Timeout  time.Duration
}

// redactedPowerCycleOptions is the safe projection of PowerCycleOptions
// returned by Redacted(). It omits Password and APIToken so any code path
// that formats the options — including slog's redactAny switch — cannot
// reach the secret bytes.
type redactedPowerCycleOptions struct {
	Endpoint string
	Username string
	Insecure bool
	Timeout  time.Duration
}

// Redacted returns a struct containing only the non-secret fields of o,
// satisfying the interface{ Redacted() any } that logutil.redactAny detects.
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

// String masks secret fields so accidental %v / %s / log calls can't leak
// the password or token.
func (o *PowerCycleOptions) String() string {
	if o == nil {
		return "PowerCycleOptions(nil)"
	}
	return fmt.Sprintf("%+v", o.Redacted())
}

// PowerCycler stops then starts a VM through the Proxmox API. A resize changes a
// VM's *configured* memory but bpg/proxmox does not restart it, so the guest
// keeps its old RAM until a hypervisor stop→start spawns a fresh QEMU process
// (a guest reboot reuses the same process and RAM, so it will not do). This is a
// narrow, operational power-cycle — not an infra-state mutation — and is the
// sanctioned API-path analogue of the SSH exemption in hostssh/iso_cleanup.go:
// the bastion cannot SSH to the Proxmox host, so this goes over the API instead.
// The same rationale extends to ShutdownVM/StartVM: cluster stop/start needs
// graceful, per-VM power control that terraform apply/destroy cannot express.
type PowerCycler struct {
	opts *PowerCycleOptions
}

// NewPowerCycler returns a PowerCycler bound to the given API credentials.
func NewPowerCycler(opts *PowerCycleOptions) *PowerCycler {
	return &PowerCycler{opts: opts}
}

// timeout returns the configured per-task timeout, or defaultPowerCycleTimeout.
func (pc *PowerCycler) timeout() time.Duration {
	if pc.opts.Timeout > 0 {
		return pc.opts.Timeout
	}
	return defaultPowerCycleTimeout
}

// vm builds a client scoped to timeout and returns the target VM with its
// current status already populated (go-proxmox fetches status/current +
// config as part of the lookup).
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

// PowerCycleVM stops (if running) then starts the VM, waiting for each task to
// complete. It is fail-closed: any error leaves the caller to treat the resize
// as unrealized. node is the Proxmox node name; vmid the QEMU id.
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

// ShutdownVM sends an ACPI graceful shutdown and waits for it to power off.
// It is a no-op if the VM already reports stopped. A completed shutdown task
// does not by itself prove the guest powered off — the guest can ignore the
// ACPI signal — so ShutdownVM re-pings the VM afterward and errors unless
// Proxmox now reports it stopped.
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

// StartVM starts the VM if it is not already running and waits for the task
// to complete. It is a no-op if the VM already reports running.
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
