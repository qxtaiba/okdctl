package proxmox

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	taskPollInterval = time.Second
	taskRunning      = "running"
	waitTaskRetries  = 5
)

func (d *driver) Start(ctx context.Context, ref VMRef) (Task, error) {
	return d.task(ctx, http.MethodPost, ref.Node, vmPath(ref, "/status/start"), nil)
}

// Shutdown asks the guest to power off via ACPI; with force the task falls
// back to a hard stop once timeout elapses instead of failing.
func (d *driver) Shutdown(ctx context.Context, ref VMRef, timeout time.Duration, force bool) (Task, error) {
	form := url.Values{}
	if timeout > 0 {
		form.Set("timeout", strconv.Itoa(int((timeout+time.Second-1)/time.Second)))
	}
	if force {
		form.Set("forceStop", "1")
	}
	return d.task(ctx, http.MethodPost, ref.Node, vmPath(ref, "/status/shutdown"), form)
}

func (d *driver) Stop(ctx context.Context, ref VMRef) (Task, error) {
	return d.task(ctx, http.MethodPost, ref.Node, vmPath(ref, "/status/stop"), nil)
}

func (d *driver) Reset(ctx context.Context, ref VMRef) (Task, error) {
	return d.task(ctx, http.MethodPost, ref.Node, vmPath(ref, "/status/reset"), nil)
}

func (d *driver) Status(ctx context.Context, ref VMRef) (VMStatus, error) {
	var raw struct {
		Status    string `json:"status"`
		QMPStatus string `json:"qmpstatus"`
		Lock      string `json:"lock"`
		HA        struct {
			Managed num `json:"managed"`
		} `json:"ha"`
	}
	path := vmPath(ref, "/status/current")
	if err := d.get(ctx, path, &raw); err != nil {
		return VMStatus{}, err
	}
	if raw.Status == "" {
		return VMStatus{}, fmt.Errorf("get %s: vm %d: %w", path, ref.VMID, ErrNotFound)
	}
	return VMStatus{Status: raw.Status, QMPStatus: raw.QMPStatus, Lock: raw.Lock, HA: raw.HA.Managed != 0}, nil
}

// WaitTask polls the task until it stops; a zero Task is already done, and a
// task ending with anything but OK or WARNINGS: n fails with *TaskFailed. A
// poll error other than permission or not-found is retried on the next tick
// up to waitTaskRetries times in a row.
func (d *driver) WaitTask(ctx context.Context, task Task) error {
	if task.UPID == "" {
		return nil
	}
	path := nodePath(task.Node, "tasks/"+url.PathEscape(task.UPID)+"/status")
	ticker := time.NewTicker(d.pollInterval)
	defer ticker.Stop()
	failures := 0
	for {
		var st struct {
			Status     string `json:"status"`
			ExitStatus string `json:"exitstatus"`
		}
		err := d.get(ctx, path, &st)
		switch {
		case err == nil:
			failures = 0
		case ctx.Err() != nil, errors.Is(err, ErrPermission), errors.Is(err, ErrNotFound):
			return err
		default:
			failures++
			if failures > waitTaskRetries {
				return err
			}
		}
		if err == nil && st.Status != taskRunning {
			// PVE's own upid_status_is_error also counts "WARNINGS: n" as success.
			if st.ExitStatus == "OK" || strings.HasPrefix(st.ExitStatus, "WARNINGS: ") {
				return nil
			}
			return &TaskFailed{UPID: task.UPID, ExitStatus: st.ExitStatus}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
