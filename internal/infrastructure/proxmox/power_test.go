package proxmox

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakePVE is an httptest-backed Proxmox API serving exactly the routes the
// PowerCycler walks: node status, VM status/current + config, the power
// POSTs, and task status. Power POSTs are recorded in order and flip the
// reported VM status so sequencing (stop before start, no start after a
// failed stop) is observable from the outside.
type fakePVE struct {
	mu             sync.Mutex
	vmStatus       string
	posts          []string
	lastUPID       string
	failAction     string // this power action's POST returns 500
	ignoreShutdown bool   // guest ignores ACPI: shutdown completes but status stays running
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
		// Mapping through constants also breaks gosec's G705 taint chain:
		// the response never echoes request-derived input.
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
		fail := action == f.failAction
		if !fail && (action != actShutdown || !f.ignoreShutdown) {
			f.vmStatus = newStatus
		}
		f.mu.Unlock()
		if fail {
			http.Error(w, "simulated task spawn failure", http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, `{"data":%q}`, upid)
	})
	// The body must carry upid and node: Task.UnmarshalJSON overwrites every
	// exported field, so omitting them zeroes t.UPID and the next poll builds
	// a bad URL (go-proxmox v0.8.1 tasks.go).
	mux.HandleFunc("GET /api2/json/nodes/pve1/tasks/{upid}/status", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		upid := f.lastUPID
		f.mu.Unlock()
		fmt.Fprintf(w, `{"data":{"upid":%q,"node":"pve1","status":"stopped","exitstatus":"OK"}}`, upid)
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

func TestPowerCycler_timeout(t *testing.T) {
	t.Run("default when unset", func(t *testing.T) {
		pc := NewPowerCycler(&PowerCycleOptions{})
		if got := pc.timeout(); got != defaultPowerCycleTimeout {
			t.Errorf("timeout() = %v; want %v", got, defaultPowerCycleTimeout)
		}
	})

	t.Run("default when negative", func(t *testing.T) {
		pc := NewPowerCycler(&PowerCycleOptions{Timeout: -time.Second})
		if got := pc.timeout(); got != defaultPowerCycleTimeout {
			t.Errorf("timeout() = %v; want %v", got, defaultPowerCycleTimeout)
		}
	})

	t.Run("override honored", func(t *testing.T) {
		want := 90 * time.Second
		pc := NewPowerCycler(&PowerCycleOptions{Timeout: want})
		if got := pc.timeout(); got != want {
			t.Errorf("timeout() = %v; want %v", got, want)
		}
	})
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
