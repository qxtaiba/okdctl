package proxmox

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var ref101 = VMRef{Node: "pve", VMID: 101}

func intp(v int) *int    { return &v }
func boolp(v bool) *bool { return &v }

func fastDriver(t *testing.T, f *fakeAPI) Driver {
	t.Helper()
	d := f.driver(t)
	d.(*driver).pollInterval = 5 * time.Millisecond
	return d
}

func newSpec(vmid int, tag string) VMSpec {
	return VMSpec{
		VMID: vmid, Name: "grappleberry-worker-0", CPUType: "host", Cores: 4, MemoryMiB: 16384,
		Disks:      []DiskSpec{{Slot: "scsi0", Storage: "local-lvm", SizeGiB: 120, Serial: "worker-0-root"}},
		EFIStorage: "local-lvm", MAC: "BC:24:11:AA:BB:CC", Bridge: "vmbr0",
		ISOVolID: "local:iso/grappleberry-worker.iso", Tag: tag,
	}
}

func TestCreateVM(t *testing.T) {
	t.Run("new vmid posts the spec", func(t *testing.T) {
		f := newFakeAPI(t, nil)
		spec := newSpec(105, "grappleberry")
		task, err := f.driver(t).CreateVM(t.Context(), "pve", spec)
		if err != nil {
			t.Fatalf("CreateVM(): %v", err)
		}
		if want := (Task{Node: "pve", UPID: upidCreate}); task != want {
			t.Fatalf("CreateVM() = %+v, want %+v", task, want)
		}
		if got := f.form("POST /nodes/pve/qemu"); !reflect.DeepEqual(got, spec.values()) {
			t.Fatalf("posted form =\n%v\nwant\n%v", got, spec.values())
		}
	})

	t.Run("existing vmid with our tag is a no-op", func(t *testing.T) {
		f := newFakeAPI(t, nil)
		task, err := f.driver(t).CreateVM(t.Context(), "pve", newSpec(101, "grappleberry"))
		if err != nil {
			t.Fatalf("CreateVM(): %v", err)
		}
		if task != (Task{}) {
			t.Fatalf("CreateVM() = %+v, want zero Task", task)
		}
		if f.called("POST /nodes/pve/qemu") {
			t.Fatal("CreateVM() posted a second VM for an existing vmid")
		}
	})

	t.Run("existing vmid with another tag is an error", func(t *testing.T) {
		f := newFakeAPI(t, nil)
		_, err := f.driver(t).CreateVM(t.Context(), "pve", newSpec(200, "grappleberry"))
		if err == nil || !strings.Contains(err.Error(), "200") || !strings.Contains(err.Error(), "lab-9f3a1c") {
			t.Fatalf("CreateVM() error = %v, want it to name vm 200 and its tag", err)
		}
		if f.called("POST /nodes/pve/qemu") {
			t.Fatal("CreateVM() posted over a foreign VM")
		}
	})

	t.Run("prefix tag does not match", func(t *testing.T) {
		f := newFakeAPI(t, nil)
		if _, err := f.driver(t).CreateVM(t.Context(), "pve", newSpec(201, "lab-9f3a1c")); err == nil {
			t.Fatal("CreateVM() treated tag lab-9f3a1cx as lab-9f3a1c")
		}
	})
}

func TestUpdateVM(t *testing.T) {
	f := newFakeAPI(t, nil)
	patch := VMPatch{Cores: intp(8), MemoryMiB: intp(32768), Tags: []string{"grappleberry", "okd"}, OnBoot: boolp(false)}
	task, err := f.driver(t).UpdateVM(t.Context(), ref101, patch, "7f3a0c2e")
	if err != nil {
		t.Fatalf("UpdateVM(): %v", err)
	}
	if want := (Task{Node: "pve", UPID: upidConfig}); task != want {
		t.Fatalf("UpdateVM() = %+v, want %+v", task, want)
	}
	want := url.Values{"digest": {"7f3a0c2e"}, "cores": {"8"}, "memory": {"32768"}, "tags": {"grappleberry;okd"}, "onboot": {"0"}}
	if got := f.form("POST /nodes/pve/qemu/101/config"); !reflect.DeepEqual(got, want) {
		t.Fatalf("posted form = %v, want %v", got, want)
	}

	t.Run("nil fields are left out", func(t *testing.T) {
		f := newFakeAPI(t, nil)
		if _, err := f.driver(t).UpdateVM(t.Context(), ref101, VMPatch{Cores: intp(2)}, "d1"); err != nil {
			t.Fatalf("UpdateVM(): %v", err)
		}
		want := url.Values{"digest": {"d1"}, "cores": {"2"}}
		if got := f.form("POST /nodes/pve/qemu/101/config"); !reflect.DeepEqual(got, want) {
			t.Fatalf("posted form = %v, want %v", got, want)
		}
	})
}

func TestResizeDisk(t *testing.T) {
	f := newFakeAPI(t, nil)
	task, err := f.driver(t).ResizeDisk(t.Context(), ref101, "scsi0", "+10G")
	if err != nil {
		t.Fatalf("ResizeDisk(): %v", err)
	}
	if want := (Task{Node: "pve", UPID: upidResize}); task != want {
		t.Fatalf("ResizeDisk() = %+v, want %+v", task, want)
	}
	want := url.Values{"disk": {"scsi0"}, "size": {"+10G"}}
	if got := f.form("PUT /nodes/pve/qemu/101/resize"); !reflect.DeepEqual(got, want) {
		t.Fatalf("put form = %v, want %v", got, want)
	}
}

func TestDetachISO(t *testing.T) {
	f := newFakeAPI(t, nil)
	task, err := f.driver(t).DetachISO(t.Context(), ref101)
	if err != nil {
		t.Fatalf("DetachISO(): %v", err)
	}
	if want := (Task{Node: "pve", UPID: upidConfig}); task != want {
		t.Fatalf("DetachISO() = %+v, want %+v", task, want)
	}
	want := url.Values{"ide2": {"none,media=cdrom"}, "boot": {"order=scsi0"}}
	if got := f.form("POST /nodes/pve/qemu/101/config"); !reflect.DeepEqual(got, want) {
		t.Fatalf("posted form = %v, want %v", got, want)
	}
}

func TestDeleteVM(t *testing.T) {
	f := newFakeAPI(t, nil)
	task, err := f.driver(t).DeleteVM(t.Context(), ref101)
	if err != nil {
		t.Fatalf("DeleteVM(): %v", err)
	}
	if want := (Task{Node: "pve", UPID: upidDestroy}); task != want {
		t.Fatalf("DeleteVM() = %+v, want %+v", task, want)
	}
	if !f.called("DELETE /nodes/pve/qemu/101?purge=1&destroy-unreferenced-disks=1") {
		t.Fatalf("DeleteVM() calls = %+v, want a purging delete", f.calls)
	}
}

func TestPowerActions(t *testing.T) {
	cases := []struct {
		name string
		run  func(Driver) (Task, error)
		key  string
		upid string
	}{
		{"start", func(d Driver) (Task, error) { return d.Start(t.Context(), ref101) }, "POST /nodes/pve/qemu/101/status/start", upidStart},
		{"stop", func(d Driver) (Task, error) { return d.Stop(t.Context(), ref101) }, "POST /nodes/pve/qemu/101/status/stop", upidStop},
		{"reset", func(d Driver) (Task, error) { return d.Reset(t.Context(), ref101) }, "POST /nodes/pve/qemu/101/status/reset", upidReset},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeAPI(t, nil)
			task, err := tc.run(f.driver(t))
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if want := (Task{Node: "pve", UPID: tc.upid}); task != want {
				t.Fatalf("%s = %+v, want %+v", tc.name, task, want)
			}
			if !f.called(tc.key) {
				t.Fatalf("%s did not post %s; calls: %+v", tc.name, tc.key, f.calls)
			}
		})
	}
}

func TestShutdown(t *testing.T) {
	const key = "POST /nodes/pve/qemu/101/status/shutdown"
	t.Run("graceful", func(t *testing.T) {
		f := newFakeAPI(t, nil)
		task, err := f.driver(t).Shutdown(t.Context(), ref101, 30*time.Second, false)
		if err != nil {
			t.Fatalf("Shutdown(): %v", err)
		}
		if want := (Task{Node: "pve", UPID: upidShutdown}); task != want {
			t.Fatalf("Shutdown() = %+v, want %+v", task, want)
		}
		if got, want := f.form(key), (url.Values{"timeout": {"30"}}); !reflect.DeepEqual(got, want) {
			t.Fatalf("posted form = %v, want %v", got, want)
		}
	})
	t.Run("forced", func(t *testing.T) {
		f := newFakeAPI(t, nil)
		if _, err := f.driver(t).Shutdown(t.Context(), ref101, 30*time.Second, true); err != nil {
			t.Fatalf("Shutdown(): %v", err)
		}
		if got, want := f.form(key), (url.Values{"timeout": {"30"}, "forceStop": {"1"}}); !reflect.DeepEqual(got, want) {
			t.Fatalf("posted form = %v, want %v", got, want)
		}
	})
}

func TestStatus(t *testing.T) {
	f := newFakeAPI(t, nil)
	got, err := f.driver(t).Status(t.Context(), ref101)
	if err != nil {
		t.Fatalf("Status(): %v", err)
	}
	want := VMStatus{Status: "running", QMPStatus: "running", HA: true}
	if got != want {
		t.Fatalf("Status() = %+v, want %+v", got, want)
	}

	t.Run("null data is not found", func(t *testing.T) {
		f := newFakeAPI(t, map[string]http.HandlerFunc{
			"GET /nodes/pve/qemu/999/status/current": jsonHandler(http.StatusOK, `{"data":null}`),
		})
		_, err := f.driver(t).Status(t.Context(), VMRef{Node: "pve", VMID: 999})
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("Status() error = %v, want ErrNotFound", err)
		}
	})
}

func taskStatusHandler(states ...string) (http.HandlerFunc, *atomic.Int32) {
	var polls atomic.Int32
	return func(w http.ResponseWriter, _ *http.Request) {
		i := int(polls.Add(1)) - 1
		if i >= len(states) {
			i = len(states) - 1
		}
		w.Header().Set("Content-Type", "application/json")
		if states[i] == "running" {
			_, _ = w.Write([]byte(`{"data":{"upid":"` + upidShutdown + `","node":"pve","status":"running"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"upid":"` + upidShutdown + `","node":"pve","status":"stopped","exitstatus":"` + states[i] + `"}}`))
	}, &polls
}

func TestWaitTask(t *testing.T) {
	const key = "GET /nodes/pve/tasks/" + upidShutdown + "/status"
	shutdown := Task{Node: "pve", UPID: upidShutdown}

	t.Run("fixture task is ok", func(t *testing.T) {
		f := newFakeAPI(t, nil)
		if err := f.driver(t).WaitTask(t.Context(), Task{Node: "pve", UPID: upidStart}); err != nil {
			t.Fatalf("WaitTask(): %v", err)
		}
	})

	t.Run("polls until stopped", func(t *testing.T) {
		h, polls := taskStatusHandler("running", "running", "OK")
		f := newFakeAPI(t, map[string]http.HandlerFunc{key: h})
		if err := fastDriver(t, f).WaitTask(t.Context(), shutdown); err != nil {
			t.Fatalf("WaitTask(): %v", err)
		}
		if got := polls.Load(); got != 3 {
			t.Fatalf("polled %d times, want 3", got)
		}
	})

	t.Run("warnings are success", func(t *testing.T) {
		h, _ := taskStatusHandler("WARNINGS: 2")
		f := newFakeAPI(t, map[string]http.HandlerFunc{key: h})
		if err := fastDriver(t, f).WaitTask(t.Context(), shutdown); err != nil {
			t.Fatalf("WaitTask(): %v", err)
		}
	})

	t.Run("failure carries the exit status", func(t *testing.T) {
		const exit = "VM quit/powerdown failed - got timeout"
		h, _ := taskStatusHandler("running", exit)
		f := newFakeAPI(t, map[string]http.HandlerFunc{key: h})
		err := fastDriver(t, f).WaitTask(t.Context(), shutdown)
		var failed *TaskFailed
		if !errors.As(err, &failed) {
			t.Fatalf("WaitTask() error = %v (%T), want *TaskFailed", err, err)
		}
		if failed.UPID != upidShutdown || failed.ExitStatus != exit {
			t.Fatalf("TaskFailed = %+v, want UPID %q and exit status %q", failed, upidShutdown, exit)
		}
		if !strings.Contains(err.Error(), exit) {
			t.Fatalf("error %q does not carry the exit status", err)
		}
	})

	t.Run("zero task is already done", func(t *testing.T) {
		f := newFakeAPI(t, nil)
		if err := f.driver(t).WaitTask(t.Context(), Task{}); err != nil {
			t.Fatalf("WaitTask(zero): %v", err)
		}
		if len(f.calls) != 0 {
			t.Fatalf("WaitTask(zero) made calls: %+v", f.calls)
		}
	})

	t.Run("honours context", func(t *testing.T) {
		h, _ := taskStatusHandler("running")
		f := newFakeAPI(t, map[string]http.HandlerFunc{key: h})
		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()
		began := time.Now()
		err := fastDriver(t, f).WaitTask(ctx, shutdown)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("WaitTask() error = %v, want context.DeadlineExceeded", err)
		}
		if waited := time.Since(began); waited > time.Second {
			t.Fatalf("WaitTask() returned after %v, want promptly after cancellation", waited)
		}
	})
}

func TestHA(t *testing.T) {
	t.Run("set with group", func(t *testing.T) {
		f := newFakeAPI(t, nil)
		if err := f.driver(t).SetHA(t.Context(), ref101, HASpec{State: "started", Group: "okd"}); err != nil {
			t.Fatalf("SetHA(): %v", err)
		}
		want := url.Values{"sid": {"vm:101"}, "state": {"started"}, "group": {"okd"}}
		if got := f.form("POST /cluster/ha/resources"); !reflect.DeepEqual(got, want) {
			t.Fatalf("posted form = %v, want %v", got, want)
		}
	})
	t.Run("set without group", func(t *testing.T) {
		f := newFakeAPI(t, nil)
		if err := f.driver(t).SetHA(t.Context(), ref101, HASpec{State: "started"}); err != nil {
			t.Fatalf("SetHA(): %v", err)
		}
		want := url.Values{"sid": {"vm:101"}, "state": {"started"}}
		if got := f.form("POST /cluster/ha/resources"); !reflect.DeepEqual(got, want) {
			t.Fatalf("posted form = %v, want %v", got, want)
		}
	})
	t.Run("clear purges", func(t *testing.T) {
		f := newFakeAPI(t, nil)
		if err := f.driver(t).ClearHA(t.Context(), ref101); err != nil {
			t.Fatalf("ClearHA(): %v", err)
		}
		if !f.called("DELETE /cluster/ha/resources/vm:101?purge=1") {
			t.Fatalf("ClearHA() calls = %+v, want a purging delete", f.calls)
		}
	})
}
