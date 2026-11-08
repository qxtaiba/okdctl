package proxmox

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

func vmPath(ref VMRef, rest string) string {
	return nodePath(ref.Node, "qemu/"+strconv.Itoa(ref.VMID)+rest)
}

func haSID(ref VMRef) string {
	return "vm:" + strconv.Itoa(ref.VMID)
}

// CreateVM is idempotent by VMID: a VM already carrying spec.Tag yields a
// zero Task and any other existing VM is an error, so a create whose
// WaitTask timed out can be retried. It probes /cluster/resources because
// PVE answers a missing VM's config read with HTTP 500, which go-proxmox
// strips of its body.
func (d *driver) CreateVM(ctx context.Context, node string, spec VMSpec) (Task, error) { //nolint:gocritic // hugeParam: Driver passes specs by value
	vms, err := d.ListVMs(ctx)
	if err != nil {
		return Task{}, fmt.Errorf("create vm %d: %w", spec.VMID, err)
	}
	if i := slices.IndexFunc(vms, func(vm VMSummary) bool { return vm.VMID == spec.VMID }); i >= 0 {
		vm := vms[i]
		if slices.Contains(vm.Tags, spec.Tag) {
			return Task{}, nil
		}
		return Task{}, fmt.Errorf("create vm %d: vm %q on %s already exists with tags %q",
			spec.VMID, vm.Name, vm.Node, strings.Join(vm.Tags, ";"))
	}
	return d.task(ctx, http.MethodPost, node, nodePath(node, "qemu"), spec.values())
}

func (d *driver) VMConfig(ctx context.Context, ref VMRef) (VMConfig, error) {
	var raw map[string]any
	path := vmPath(ref, "/config")
	if err := d.get(ctx, path, &raw); err != nil {
		return VMConfig{}, err
	}
	if len(raw) == 0 {
		return VMConfig{}, fmt.Errorf("get %s: vm %d: %w", path, ref.VMID, ErrNotFound)
	}
	cfg, err := ParseVMConfig(raw)
	if err != nil {
		return VMConfig{}, fmt.Errorf("get %s: %w", path, err)
	}
	return cfg, nil
}

type pendingRow struct {
	Key     string `json:"key"`
	Value   any    `json:"value"`
	Pending any    `json:"pending"`
	Delete  num    `json:"delete"`
}

// VMPending returns only the keys with a queued value or deletion; PVE lists
// every current key there, so an empty reply means the VM does not exist.
func (d *driver) VMPending(ctx context.Context, ref VMRef) ([]Pending, error) {
	var rows []pendingRow
	path := vmPath(ref, "/pending")
	if err := d.get(ctx, path, &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("get %s: vm %d: %w", path, ref.VMID, ErrNotFound)
	}
	var pending []Pending
	for _, r := range rows {
		if r.Pending == nil && r.Delete == 0 {
			continue
		}
		pending = append(pending, Pending{Key: r.Key, Value: scalar(r.Value), Pending: scalar(r.Pending), Delete: r.Delete != 0})
	}
	return pending, nil
}

func (d *driver) UpdateVM(ctx context.Context, ref VMRef, patch VMPatch, digest string) (Task, error) {
	form := url.Values{"digest": {digest}}
	if patch.Cores != nil {
		form.Set("cores", strconv.Itoa(*patch.Cores))
	}
	if patch.MemoryMiB != nil {
		form.Set("memory", strconv.Itoa(*patch.MemoryMiB))
	}
	if patch.Tags != nil {
		form.Set("tags", strings.Join(patch.Tags, ";"))
	}
	if patch.OnBoot != nil {
		form.Set("onboot", boolFlag(*patch.OnBoot))
	}
	if len(patch.Delete) > 0 {
		form.Set("delete", strings.Join(patch.Delete, ","))
	}
	return d.task(ctx, http.MethodPost, ref.Node, vmPath(ref, "/config"), form)
}

func (d *driver) ResizeDisk(ctx context.Context, ref VMRef, disk, size string) (Task, error) {
	return d.task(ctx, http.MethodPut, ref.Node, vmPath(ref, "/resize"), url.Values{"disk": {disk}, "size": {size}})
}

func (d *driver) DetachISO(ctx context.Context, ref VMRef) (Task, error) {
	form := url.Values{cdromSlot: {"none,media=cdrom"}, "boot": {"order=" + bootDisk}}
	return d.task(ctx, http.MethodPost, ref.Node, vmPath(ref, "/config"), form)
}

func (d *driver) DeleteVM(ctx context.Context, ref VMRef) (Task, error) {
	return d.task(ctx, http.MethodDelete, ref.Node, vmPath(ref, "?purge=1&destroy-unreferenced-disks=1"), nil)
}

// SetHA upserts the HA resource: a registered VM is updated in place, with
// an empty Group deleting any group it had.
func (d *driver) SetHA(ctx context.Context, ref VMRef, spec HASpec) error {
	sid := haSID(ref)
	path := "/cluster/ha/resources/" + url.PathEscape(sid)
	var current struct {
		SID string `json:"sid"`
	}
	err := d.get(ctx, path, &current)
	switch {
	case errors.Is(err, ErrNotFound) || (err == nil && current.SID == ""):
		form := url.Values{"sid": {sid}, "state": {spec.State}}
		if spec.Group != "" {
			form.Set("group", spec.Group)
		}
		return d.write(ctx, http.MethodPost, "/cluster/ha/resources", form, nil)
	case err != nil:
		return err
	}
	form := url.Values{"state": {spec.State}}
	if spec.Group != "" {
		form.Set("group", spec.Group)
	} else {
		form.Set("delete", "group")
	}
	return d.write(ctx, http.MethodPut, path, form, nil)
}

func (d *driver) ClearHA(ctx context.Context, ref VMRef) error {
	return d.write(ctx, http.MethodDelete, "/cluster/ha/resources/"+url.PathEscape(haSID(ref))+"?purge=1", nil, nil)
}

func boolFlag(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
