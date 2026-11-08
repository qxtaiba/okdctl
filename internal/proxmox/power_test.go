package proxmox

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakePVE serves exactly the routes PowerCycler walks, recording power POSTs in order.
type fakePVE struct {
	mu             sync.Mutex
	vmStatus       string
	posts          []string
	lastUPID       string
	lastAction     string
	failAction     string // this power action's POST returns 500
	ignoreShutdown bool   // guest ignores ACPI: shutdown completes but status stays running

	taskExitByAction map[string]string
	taskNeverEnds    bool
}

const (
	fakeNode  = "pve1"
	fakeVMID  = 101
	fakeToken = "root@pam!tok=secret"

	actStart    = "start"
	actStop     = "stop"
	actShutdown = "shutdown"

	upidStart    = "UPID:pve1:00001234:00005678:66aabbcc:qmstart:101:root@pam!tok:"
	upidStop     = "UPID:pve1:00001234:00005678:66aabbcc:qmstop:101:root@pam!tok:"
	upidShutdown = "UPID:pve1:00001234:00005678:66aabbcc:qmshutdown:101:root@pam!tok:"
)

func (f *fakePVE) actions() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.posts)
}

func (f *fakePVE) start(t *testing.T) *PowerCycler {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api2/json/cluster/resources", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"data":[{"vmid":101,"type":"qemu","node":"pve1"}]}`)
	})
	mux.HandleFunc("GET /api2/json/nodes/pve1/status", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"data":{}}`)
	})
	mux.HandleFunc("GET /api2/json/nodes/pve1/qemu/101/status/current", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		st := f.vmStatus
		f.mu.Unlock()
		fmt.Fprintf(w, `{"data":{"vmid":101,"status":%q}}`, st)
	})
	mux.HandleFunc("GET /api2/json/nodes/pve1/qemu/101/config", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"data":{}}`)
	})
	mux.HandleFunc("POST /api2/json/nodes/pve1/qemu/101/status/{action}", func(w http.ResponseWriter, r *http.Request) {
		// Mapping through constants also breaks gosec's G705 taint chain.
		var action, upid, newStatus string
		switch r.PathValue("action") {
		case actStart:
			action, upid, newStatus = actStart, upidStart, "running"
		case actStop:
			action, upid, newStatus = actStop, upidStop, "stopped"
		case actShutdown:
			action, upid, newStatus = actShutdown, upidShutdown, "stopped"
		default:
			http.NotFound(w, r)
			return
		}
		f.mu.Lock()
		f.posts = append(f.posts, action)
		f.lastUPID = upid
		f.lastAction = action
		fail := action == f.failAction
		_, taskFails := f.taskExitByAction[action]
		if !fail && !taskFails && !f.taskNeverEnds && (action != actShutdown || !f.ignoreShutdown) {
			f.vmStatus = newStatus
		}
		f.mu.Unlock()
		if fail {
			http.Error(w, "simulated task spawn failure", http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, `{"data":%q}`, upid)
	})
	// upid/node must be present: Task.UnmarshalJSON overwrites every field,
	// zeroing t.UPID and breaking the next poll's URL (go-proxmox v0.8.1 tasks.go).
	mux.HandleFunc("GET /api2/json/nodes/pve1/tasks/{upid}/status", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		upid := f.lastUPID
		exit, taskFails := f.taskExitByAction[f.lastAction]
		running := f.taskNeverEnds
		f.mu.Unlock()
		if running {
			fmt.Fprintf(w, `{"data":{"upid":%q,"node":"pve1","status":"running"}}`, upid)
			return
		}
		if !taskFails {
			exit = "OK"
		}
		fmt.Fprintf(w, `{"data":{"upid":%q,"node":"pve1","status":"stopped","exitstatus":%q}}`, upid, exit)
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "PVEAPIToken="+fakeToken {
			t.Errorf("Authorization = %q; want token auth on every request", got)
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)

	return NewPowerCycler(&PowerCycleOptions{
		Endpoint: srv.URL,
		APIToken: []byte(fakeToken),
		Timeout:  5 * time.Second,
	})
}

func TestPowerCyclerPowerCycleVM(t *testing.T) {
	t.Run("running vm stops then starts", func(t *testing.T) {
		f := &fakePVE{vmStatus: "running"}
		pc := f.start(t)
		if err := pc.PowerCycleVM(context.Background(), fakeNode, fakeVMID); err != nil {
			t.Fatalf("PowerCycleVM: %v", err)
		}
		if got, want := f.actions(), []string{actStop, actStart}; !slices.Equal(got, want) {
			t.Errorf("power actions = %v; want %v", got, want)
		}
	})

	t.Run("stopped vm skips the stop", func(t *testing.T) {
		f := &fakePVE{vmStatus: "stopped"}
		pc := f.start(t)
		if err := pc.PowerCycleVM(context.Background(), fakeNode, fakeVMID); err != nil {
			t.Fatalf("PowerCycleVM: %v", err)
		}
		if got, want := f.actions(), []string{actStart}; !slices.Equal(got, want) {
			t.Errorf("power actions = %v; want %v", got, want)
		}
	})

	t.Run("failed stop aborts before start", func(t *testing.T) {
		f := &fakePVE{vmStatus: "running", failAction: actStop}
		pc := f.start(t)
		err := pc.PowerCycleVM(context.Background(), fakeNode, fakeVMID)
		if err == nil || !strings.Contains(err.Error(), "stop vm 101") {
			t.Fatalf("PowerCycleVM err = %v; want stop vm 101 wrap", err)
		}
		if got, want := f.actions(), []string{actStop}; !slices.Equal(got, want) {
			t.Errorf("power actions = %v; want %v (no start after failed stop)", got, want)
		}
	})
}

func TestPowerCyclerShutdownVM(t *testing.T) {
	t.Run("running vm shuts down", func(t *testing.T) {
		f := &fakePVE{vmStatus: "running"}
		pc := f.start(t)
		if err := pc.ShutdownVM(context.Background(), fakeNode, fakeVMID); err != nil {
			t.Fatalf("ShutdownVM: %v", err)
		}
		if got, want := f.actions(), []string{actShutdown}; !slices.Equal(got, want) {
			t.Errorf("power actions = %v; want %v", got, want)
		}
	})

	t.Run("stopped vm is a no-op", func(t *testing.T) {
		f := &fakePVE{vmStatus: "stopped"}
		pc := f.start(t)
		if err := pc.ShutdownVM(context.Background(), fakeNode, fakeVMID); err != nil {
			t.Fatalf("ShutdownVM: %v", err)
		}
		if got := f.actions(); len(got) != 0 {
			t.Errorf("power actions = %v; want none", got)
		}
	})

	t.Run("guest ignoring acpi is an error", func(t *testing.T) {
		f := &fakePVE{vmStatus: "running", ignoreShutdown: true}
		pc := f.start(t)
		err := pc.ShutdownVM(context.Background(), fakeNode, fakeVMID)
		if err == nil || !strings.Contains(err.Error(), "still running after shutdown task completed") {
			t.Fatalf("ShutdownVM err = %v; want still-running confirmation error", err)
		}
	})
}

func TestPowerCyclerStartVM(t *testing.T) {
	t.Run("stopped vm starts", func(t *testing.T) {
		f := &fakePVE{vmStatus: "stopped"}
		pc := f.start(t)
		if err := pc.StartVM(context.Background(), fakeNode, fakeVMID); err != nil {
			t.Fatalf("StartVM: %v", err)
		}
		if got, want := f.actions(), []string{actStart}; !slices.Equal(got, want) {
			t.Errorf("power actions = %v; want %v", got, want)
		}
	})

	t.Run("running vm is a no-op", func(t *testing.T) {
		f := &fakePVE{vmStatus: "running"}
		pc := f.start(t)
		if err := pc.StartVM(context.Background(), fakeNode, fakeVMID); err != nil {
			t.Fatalf("StartVM: %v", err)
		}
		if got := f.actions(); len(got) != 0 {
			t.Errorf("power actions = %v; want none", got)
		}
	})
}

func TestPowerCyclerFailedTaskIsAnError(t *testing.T) {
	const lockTimeout = "can't lock file '/var/lock/qemu-server/lock-101.conf' - got timeout"
	cases := []struct {
		name        string
		vmStatus    string
		failing     string
		run         func(*PowerCycler) error
		wantWrap    string
		wantActions []string
	}{
		{
			name: "power-cycle stop task", vmStatus: "running", failing: actStop,
			run:      func(pc *PowerCycler) error { return pc.PowerCycleVM(t.Context(), fakeNode, fakeVMID) },
			wantWrap: "wait for vm 101 stop", wantActions: []string{actStop},
		},
		{
			name: "power-cycle start task", vmStatus: "running", failing: actStart,
			run:      func(pc *PowerCycler) error { return pc.PowerCycleVM(t.Context(), fakeNode, fakeVMID) },
			wantWrap: "wait for vm 101 start", wantActions: []string{actStop, actStart},
		},
		{
			name: "shutdown task", vmStatus: "running", failing: actShutdown,
			run:      func(pc *PowerCycler) error { return pc.ShutdownVM(t.Context(), fakeNode, fakeVMID) },
			wantWrap: "wait for vm 101 shutdown", wantActions: []string{actShutdown},
		},
		{
			name: "start task", vmStatus: "stopped", failing: actStart,
			run:      func(pc *PowerCycler) error { return pc.StartVM(t.Context(), fakeNode, fakeVMID) },
			wantWrap: "wait for vm 101 start", wantActions: []string{actStart},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakePVE{vmStatus: tc.vmStatus, taskExitByAction: map[string]string{tc.failing: lockTimeout}}
			err := tc.run(f.start(t))
			if err == nil {
				t.Fatalf("err = nil; want the failed %s task reported", tc.failing)
			}
			if !strings.Contains(err.Error(), tc.wantWrap) || !strings.Contains(err.Error(), lockTimeout) {
				t.Errorf("err = %v; want %q and the task exit status %q", err, tc.wantWrap, lockTimeout)
			}
			if got := f.actions(); !slices.Equal(got, tc.wantActions) {
				t.Errorf("power actions = %v; want %v", got, tc.wantActions)
			}
		})
	}
}

func TestPowerCyclerTaskWithWarningsSucceeds(t *testing.T) {
	f := &fakePVE{vmStatus: "stopped", taskExitByAction: map[string]string{actStart: "WARNINGS: 1"}}
	if err := f.start(t).StartVM(t.Context(), fakeNode, fakeVMID); err != nil {
		t.Fatalf("StartVM: %v", err)
	}
}

func TestPowerCyclerTaskWaitHonoursContext(t *testing.T) {
	f := &fakePVE{vmStatus: "stopped", taskNeverEnds: true}
	pc := f.start(t)
	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(100*time.Millisecond, cancel)

	began := time.Now()
	err := pc.StartVM(ctx, fakeNode, fakeVMID)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("StartVM err = %v; want context.Canceled", err)
	}
	if waited := time.Since(began); waited >= powerTaskPollInterval {
		t.Errorf("StartVM returned after %v; want it back before the next %v poll", waited, powerTaskPollInterval)
	}
}

func TestPowerCyclerTaskWaitTimesOut(t *testing.T) {
	f := &fakePVE{vmStatus: "stopped", taskNeverEnds: true}
	pc := f.start(t)
	pc.opts.Timeout = 200 * time.Millisecond

	began := time.Now()
	err := pc.StartVM(t.Context(), fakeNode, fakeVMID)
	if err == nil || !strings.Contains(err.Error(), "still running after 200ms") {
		t.Fatalf("StartVM err = %v; want a still-running timeout", err)
	}
	if waited := time.Since(began); waited >= powerTaskPollInterval {
		t.Errorf("StartVM returned after %v; want it back at the 200ms timeout", waited)
	}
}

func TestPowerCycler_timeout(t *testing.T) {
	pc := NewPowerCycler(&PowerCycleOptions{})
	if got := pc.timeout(); got != defaultPowerCycleTimeout {
		t.Errorf("unset timeout() = %v; want %v", got, defaultPowerCycleTimeout)
	}
}

func TestPowerCycleOptionsRedacted(t *testing.T) {
	o := &PowerCycleOptions{
		Endpoint: "https://pve:8006",
		Username: "root@pam",
		Password: []byte("hunter2"),
		APIToken: []byte("user@pam!t=sekrit"),
	}
	// o.String() covers %v / %s: fmt delegates both to the Stringer.
	for _, rendered := range []string{
		fmt.Sprintf("%+v", o),
		o.String(),
		fmt.Sprintf("%+v", o.Redacted()),
	} {
		if strings.Contains(rendered, "hunter2") || strings.Contains(rendered, "sekrit") {
			t.Errorf("rendered options leak a secret: %q", rendered)
		}
	}
	if !strings.Contains(o.String(), "https://pve:8006") {
		t.Errorf("String() = %q; want the non-secret endpoint retained", o.String())
	}
	if (*PowerCycleOptions)(nil).Redacted() != nil {
		t.Error("nil Redacted() should be nil")
	}
	if got := (*PowerCycleOptions)(nil).String(); got != "PowerCycleOptions(nil)" {
		t.Errorf("nil String() = %q", got)
	}
}

func TestPowerCycleResolvesMigratedVM(t *testing.T) {
	f := &fakePVE{vmStatus: "running"}
	pc := f.start(t)
	if err := pc.PowerCycleVM(t.Context(), "previous-host", fakeVMID); err != nil {
		t.Fatal(err)
	}
	if got := f.actions(); !slices.Equal(got, []string{actStop, actStart}) {
		t.Fatalf("actions=%v", got)
	}
	if _, err := pc.VMOwner(t.Context(), 999); err == nil {
		t.Fatal("missing VM owner accepted")
	}
}
