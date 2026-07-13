package node

import (
	"context"
	"sync"
	"testing"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

// recordingReporter is a logutil.ProgressReporter fake that records every
// start/stop pair so a test can assert per-step invocation counts and that
// no two spans overlap (spinners are serial-only, never nested).
type recordingReporter struct {
	mu        sync.Mutex
	starts    []string
	stops     int
	active    int
	maxActive int
}

func (rr *recordingReporter) reporter() func(string) func() {
	return func(desc string) func() {
		rr.mu.Lock()
		rr.starts = append(rr.starts, desc)
		rr.active++
		if rr.active > rr.maxActive {
			rr.maxActive = rr.active
		}
		rr.mu.Unlock()

		var once sync.Once
		return func() {
			once.Do(func() {
				rr.mu.Lock()
				rr.stops++
				rr.active--
				rr.mu.Unlock()
			})
		}
	}
}

func (rr *recordingReporter) starCount() int {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	return len(rr.starts)
}

func TestReporterInvokedDuringMasterResize(t *testing.T) {
	fc := &fakeCluster{
		nodes: []cluster.NodeDetail{
			{Name: "master0", Role: nodetypes.RoleMaster, Ready: true},
			{Name: "master1", Role: nodetypes.RoleMaster, Ready: true},
			{Name: "master2", Role: nodetypes.RoleMaster, Ready: true},
		},
		etcdHealthy: true,
	}
	ftf := &fakeTF{action: terraform.PlanActionUpdate}
	cfg := config.DefaultConfig()
	cfg.Topology.ControlPlane.MemoryMB = 12288

	r, _, _ := seedRunner(t, fc, ftf, cfg)
	r.DryRun = false
	r.Power = &fakePower{}
	rr := &recordingReporter{}
	r.Reporter = rr.reporter()

	if err := r.Resize(context.Background(), ResizeScope{Role: nodetypes.RoleMaster}, ResizeOptions{MemoryMB: 24576}); err != nil {
		t.Fatalf("resize: %v", err)
	}

	// Per master: etcd gate (pre), cordon+drain, targeted apply, power-cycle,
	// node-ready wait, etcd gate (post), ceph gate (post) — seven long steps
	// must each show visible life.
	const perMaster = 7
	if got := rr.starCount(); got != perMaster*3 {
		t.Errorf("want %d reporter starts across 3 masters, got %d: %v", perMaster*3, got, rr.starts)
	}
	if rr.stops != rr.starCount() {
		t.Errorf("every started span must stop: starts=%d stops=%d", rr.starCount(), rr.stops)
	}
	if rr.maxActive > 1 {
		t.Errorf("reporter spans must be serial, never nested: maxActive=%d", rr.maxActive)
	}
}

func TestReporterInvokedDuringRemoveWorker(t *testing.T) {
	fc := &fakeCluster{
		nodes: []cluster.NodeDetail{
			{Name: "worker0", Role: nodetypes.RoleWorker},
			{Name: "worker1", Role: nodetypes.RoleWorker},
			{Name: "worker2", Role: nodetypes.RoleWorker},
			{Name: "master0", Role: nodetypes.RoleMaster},
		},
		schedulable: true,
	}
	ftf := &fakeTF{action: terraform.PlanActionDelete}
	cfg := config.DefaultConfig()
	cfg.Topology.Workers.Count = 3

	r, _, _ := seedRunner(t, fc, ftf, cfg)
	r.DryRun = false
	r.Power = &fakePower{}
	rr := &recordingReporter{}
	r.Reporter = rr.reporter()

	if err := r.RemoveWorker(context.Background(), "worker2", RemoveOptions{}); err != nil {
		t.Fatalf("remove worker: %v", err)
	}

	// Cordon+drain, targeted plan+apply, then the post-remove ceph gate.
	if got := rr.starCount(); got != 3 {
		t.Errorf("want 3 reporter starts (cordon/drain, targeted apply, ceph gate), got %d: %v", got, rr.starts)
	}
	if rr.stops != rr.starCount() {
		t.Errorf("every started span must stop: starts=%d stops=%d", rr.starCount(), rr.stops)
	}
	if rr.maxActive > 1 {
		t.Errorf("reporter spans must be serial, never nested: maxActive=%d", rr.maxActive)
	}
}

func TestReporterInvokedDuringCompact(t *testing.T) {
	fc := &fakeCluster{
		nodes:       compactNodes(),
		schedulable: true,
		etcdHealthy: true,
	}
	ftf := &fakeTF{action: terraform.PlanActionDelete}
	cfg := config.DefaultConfig()
	cfg.Topology.Workers.Count = 2

	r, _, _ := seedRunner(t, fc, ftf, cfg)
	r.DryRun = false
	r.Power = &fakePower{}
	rr := &recordingReporter{}
	r.Reporter = rr.reporter()

	if err := r.Compact(context.Background(), CompactOptions{IngressReplicas: 2}); err != nil {
		t.Fatalf("compact: %v", err)
	}

	// compact-preflight etcd gate, the N-worker plan-gate preflight span, two
	// workers each with cordon/drain + targeted apply + post-remove ceph gate,
	// and the compact-final etcd + ceph gates.
	const want = 1 + 1 + 2*3 + 2
	if got := rr.starCount(); got != want {
		t.Errorf("want %d reporter starts, got %d: %v", want, got, rr.starts)
	}
	if rr.stops != rr.starCount() {
		t.Errorf("every started span must stop: starts=%d stops=%d", rr.starCount(), rr.stops)
	}
	if rr.maxActive > 1 {
		t.Errorf("reporter spans must be serial, never nested: maxActive=%d", rr.maxActive)
	}
}

func TestReporterSilentOnDryRun(t *testing.T) {
	t.Run("resize", func(t *testing.T) {
		fc := &fakeCluster{
			nodes:       []cluster.NodeDetail{{Name: "master0", Role: nodetypes.RoleMaster}},
			etcdHealthy: true,
		}
		ftf := &fakeTF{action: terraform.PlanActionUpdate}
		cfg := config.DefaultConfig()
		cfg.Topology.ControlPlane.MemoryMB = 12288

		r, _, _ := seedRunner(t, fc, ftf, cfg) // DryRun: true
		rr := &recordingReporter{}
		r.Reporter = rr.reporter()

		if err := r.Resize(context.Background(), ResizeScope{Role: nodetypes.RoleMaster}, ResizeOptions{MemoryMB: 24576}); err != nil {
			t.Fatalf("dry-run resize: %v", err)
		}
		if got := rr.starCount(); got != 0 {
			t.Errorf("dry-run resize must not invoke the reporter, got %d calls: %v", got, rr.starts)
		}
	})

	t.Run("remove worker", func(t *testing.T) {
		fc := &fakeCluster{
			nodes: []cluster.NodeDetail{
				{Name: "worker0", Role: nodetypes.RoleWorker},
				{Name: "master0", Role: nodetypes.RoleMaster},
			},
			schedulable: true,
		}
		ftf := &fakeTF{action: terraform.PlanActionDelete}
		cfg := config.DefaultConfig()
		cfg.Topology.Workers.Count = 1

		r, _, _ := seedRunner(t, fc, ftf, cfg) // DryRun: true
		rr := &recordingReporter{}
		r.Reporter = rr.reporter()

		if err := r.RemoveWorker(context.Background(), "worker0", RemoveOptions{}); err != nil {
			t.Fatalf("dry-run remove: %v", err)
		}
		if got := rr.starCount(); got != 0 {
			t.Errorf("dry-run remove must not invoke the reporter, got %d calls: %v", got, rr.starts)
		}
	})

	t.Run("compact", func(t *testing.T) {
		fc := &fakeCluster{
			nodes:       compactNodes(),
			schedulable: true,
			etcdHealthy: true,
		}
		ftf := &fakeTF{action: terraform.PlanActionDelete}
		cfg := config.DefaultConfig()
		cfg.Topology.Workers.Count = 2

		r, _, _ := seedRunner(t, fc, ftf, cfg) // DryRun: true
		rr := &recordingReporter{}
		r.Reporter = rr.reporter()

		// Compact's etcd preflight gate runs even under --dry-run, ahead of the
		// dry-run branch check — it must stay silent too.
		if err := r.Compact(context.Background(), CompactOptions{IngressReplicas: 2}); err != nil {
			t.Fatalf("dry-run compact: %v", err)
		}
		if got := rr.starCount(); got != 0 {
			t.Errorf("dry-run compact must not invoke the reporter, got %d calls: %v", got, rr.starts)
		}
	})
}
