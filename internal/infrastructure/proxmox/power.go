package proxmox

import (
	"context"
	"fmt"
	"time"
)

// DefaultPowerCycleTimeout bounds each stop/start task within a power-cycle.
const DefaultPowerCycleTimeout = 5 * time.Minute

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

// PowerCycler stops then starts a VM through the Proxmox API. A resize changes a
// VM's *configured* memory but bpg/proxmox does not restart it, so the guest
// keeps its old RAM until a hypervisor stop→start spawns a fresh QEMU process
// (a guest reboot reuses the same process and RAM, so it will not do). This is a
// narrow, operational power-cycle — not an infra-state mutation — and is the
// sanctioned API-path analogue of the SSH exemption in hostssh/iso_cleanup.go:
// the bastion cannot SSH to the Proxmox host, so this goes over the API instead.
type PowerCycler struct {
	opts *PowerCycleOptions
}

// NewPowerCycler returns a PowerCycler bound to the given API credentials.
func NewPowerCycler(opts *PowerCycleOptions) *PowerCycler {
	return &PowerCycler{opts: opts}
}

// PowerCycleVM stops (if running) then starts the VM, waiting for each task to
// complete. It is fail-closed: any error leaves the caller to treat the resize
// as unrealized. node is the Proxmox node name; vmid the QEMU id.
func (pc *PowerCycler) PowerCycleVM(ctx context.Context, node string, vmid int) error {
	timeout := pc.opts.Timeout
	if timeout <= 0 {
		timeout = DefaultPowerCycleTimeout
	}

	client, err := newProxmoxClient(pc.opts.Endpoint, pc.opts.Username, pc.opts.Password, pc.opts.APIToken, pc.opts.Insecure, timeout)
	if err != nil {
		return err
	}

	n, err := client.Node(ctx, node)
	if err != nil {
		return fmt.Errorf("get proxmox node %s: %w", node, err)
	}
	vm, err := n.VirtualMachine(ctx, vmid)
	if err != nil {
		return fmt.Errorf("get vm %d: %w", vmid, err)
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
