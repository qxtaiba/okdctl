package addon

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/testutil"
)

// installFakeOC adds a no-op `oc` script to PATH so the
// executor.CommandExists("oc") guard in InstallAll/InstallOne passes.
func installFakeOC(t *testing.T) {
	t.Helper()
	testutil.InstallFakeBin(t, "oc", "#!/bin/sh\nexit 0\n")
}

type stubAddon struct {
	meta        Metadata
	installErr  error
	verifyErr   error
	uninstallN  atomic.Int32
	installN    atomic.Int32
	verifyN     atomic.Int32
	uninstallEr error
	// installHook, when non-nil, runs during Install (used to cancel the
	// caller ctx mid-install).
	installHook func()
	// uninstallCtxLive records whether the last Uninstall saw an uncancelled ctx.
	uninstallCtxLive atomic.Bool
}

func (s *stubAddon) Info() Metadata { return s.meta }
func (s *stubAddon) Install(_ context.Context, _ *Environment) error {
	s.installN.Add(1)
	if s.installHook != nil {
		s.installHook()
	}
	return s.installErr
}

func (s *stubAddon) Verify(_ context.Context, _ *Environment) error {
	s.verifyN.Add(1)
	return s.verifyErr
}

func (s *stubAddon) Uninstall(ctx context.Context, _ *Environment) error {
	s.uninstallN.Add(1)
	s.uninstallCtxLive.Store(ctx.Err() == nil)
	return s.uninstallEr
}

func registerStubs(t *testing.T, stubs ...*stubAddon) {
	t.Helper()
	registry.mu.Lock()
	for _, s := range stubs {
		registry.addons[s.meta.Name] = s
		registry.order = append(registry.order, s.meta.Name)
	}
	registry.mu.Unlock()
	t.Cleanup(func() {
		registry.mu.Lock()
		for _, s := range stubs {
			delete(registry.addons, s.meta.Name)
		}
		newOrder := registry.order[:0]
		for _, name := range registry.order {
			if _, ok := registry.addons[name]; ok {
				newOrder = append(newOrder, name)
			}
		}
		registry.order = newOrder
		registry.mu.Unlock()
	})
}

func enabledCfg(names ...string) *config.Config {
	cfg := &config.Config{Addons: make(map[string]config.AddonConfig)}
	for _, n := range names {
		cfg.Addons[n] = config.AddonConfig{Enabled: true}
	}
	return cfg
}

func TestInstallAll_MiddleFailureRollsBackOnlyMiddle(t *testing.T) {
	installFakeOC(t)
	a := &stubAddon{meta: Metadata{Name: "a", Priority: 1, DisplayName: "a"}}
	b := &stubAddon{meta: Metadata{Name: "b", Priority: 2, DisplayName: "b"}, installErr: errors.New("b install fails")}
	c := &stubAddon{meta: Metadata{Name: "c", Priority: 3, DisplayName: "c"}}
	registerStubs(t, a, b, c)

	mgr := NewManager(enabledCfg("a", "b", "c"))
	err := mgr.InstallAll(context.Background())
	if err == nil {
		t.Fatal("expected aggregated error, got nil")
	}
	if a.uninstallN.Load() != 0 {
		t.Errorf("a.uninstallN = %d; want 0 (independent addon must not be rolled back)", a.uninstallN.Load())
	}
	if b.uninstallN.Load() != 1 {
		t.Errorf("b.uninstallN = %d; want 1 (failed addon must be rolled back in isolation)", b.uninstallN.Load())
	}
	if c.uninstallN.Load() != 0 {
		t.Errorf("c.uninstallN = %d; want 0 (independent addon must not be rolled back)", c.uninstallN.Load())
	}
	if c.installN.Load() != 1 {
		t.Errorf("c.installN = %d; want 1 (independent addon must still install)", c.installN.Load())
	}
}

func TestInstallAll_DependencyFailureSkipsDependent(t *testing.T) {
	installFakeOC(t)
	a := &stubAddon{meta: Metadata{Name: "a", Priority: 1, DisplayName: "a"}, installErr: errors.New("a fails")}
	b := &stubAddon{meta: Metadata{Name: "b", Priority: 2, DisplayName: "b", Dependencies: []string{"a"}}}
	registerStubs(t, a, b)

	mgr := NewManager(enabledCfg("a", "b"))
	_ = mgr.InstallAll(context.Background())

	if a.uninstallN.Load() != 1 {
		t.Errorf("a.uninstallN = %d; want 1 (failed addon rolled back)", a.uninstallN.Load())
	}
	if b.installN.Load() != 0 {
		t.Errorf("b.installN = %d; want 0 (dep failed, install must be skipped)", b.installN.Load())
	}
	if b.uninstallN.Load() != 0 {
		t.Errorf("b.uninstallN = %d; want 0 (skipped addon must not be rolled back)", b.uninstallN.Load())
	}
}

func TestInstallOne_AllOrNothingReverseRollback(t *testing.T) {
	installFakeOC(t)
	a := &stubAddon{meta: Metadata{Name: "a", Priority: 1, DisplayName: "a"}}
	b := &stubAddon{meta: Metadata{Name: "b", Priority: 2, DisplayName: "b", Dependencies: []string{"a"}}}
	c := &stubAddon{meta: Metadata{Name: "c", Priority: 3, DisplayName: "c", Dependencies: []string{"b"}}, installErr: errors.New("c fails")}
	registerStubs(t, a, b, c)

	mgr := NewManager(enabledCfg("a", "b", "c"))
	err := mgr.InstallOne(context.Background(), "c")
	if err == nil {
		t.Fatal("expected error from InstallOne; got nil")
	}

	if a.uninstallN.Load() != 1 {
		t.Errorf("a.uninstallN = %d; want 1 (all-or-nothing rollback must include a)", a.uninstallN.Load())
	}
	if b.uninstallN.Load() != 1 {
		t.Errorf("b.uninstallN = %d; want 1 (all-or-nothing rollback must include b)", b.uninstallN.Load())
	}
	if c.uninstallN.Load() != 0 {
		t.Errorf("c.uninstallN = %d; want 0 (failed addon already errored before install completed)", c.uninstallN.Load())
	}
}

func TestNewManager_DefaultsExecutor(t *testing.T) {
	mgr := NewManager(enabledCfg())
	if mgr.exec == nil {
		t.Fatal("NewManager without WithExecutor must default m.exec; got nil")
	}
}

func TestNewManager_WithExecutorPreserved(t *testing.T) {
	want := executor.New()
	mgr := NewManager(enabledCfg(), WithExecutor(want))
	if mgr.exec != want {
		t.Fatal("NewManager must preserve an explicitly supplied executor")
	}
}

func TestInstallAll_CtxCancelStopsInstall(t *testing.T) {
	installFakeOC(t)
	a := &stubAddon{meta: Metadata{Name: "a", Priority: 1, DisplayName: "a"}}
	registerStubs(t, a)

	mgr := NewManager(enabledCfg("a"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := mgr.InstallAll(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v; want errors.Is(_, context.Canceled)", err)
	}
	if a.installN.Load() != 0 {
		t.Errorf("a.installN = %d; want 0 (ctx cancelled before install)", a.installN.Load())
	}
}

// TestInstallAll_RollbackRunsAfterCtxCancel asserts the rollback Uninstall of
// a failed addon runs under a detached bounded ctx: an install that failed
// because the caller ctx was cancelled must still be unwound.
func TestInstallAll_RollbackRunsAfterCtxCancel(t *testing.T) {
	installFakeOC(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &stubAddon{
		meta:        Metadata{Name: "a", Priority: 1, DisplayName: "a"},
		installErr:  errors.New("a install fails"),
		installHook: cancel,
	}
	registerStubs(t, a)

	mgr := NewManager(enabledCfg("a"))
	if err := mgr.InstallAll(ctx); err == nil {
		t.Fatal("expected aggregated error, got nil")
	}
	if a.uninstallN.Load() != 1 {
		t.Fatalf("a.uninstallN = %d; want 1 (rollback must run after ctx cancel)", a.uninstallN.Load())
	}
	if !a.uninstallCtxLive.Load() {
		t.Error("rollback Uninstall received a cancelled ctx; want a detached live ctx")
	}
}

// TestInstallOne_ReverseRollbackRunsAfterCtxCancel is the InstallOne sibling:
// the reverse-order unwind of previously-installed addons must survive a
// mid-install ctx cancellation.
func TestInstallOne_ReverseRollbackRunsAfterCtxCancel(t *testing.T) {
	installFakeOC(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &stubAddon{meta: Metadata{Name: "a", Priority: 1, DisplayName: "a"}}
	b := &stubAddon{
		meta:        Metadata{Name: "b", Priority: 2, DisplayName: "b", Dependencies: []string{"a"}},
		installErr:  errors.New("b install fails"),
		installHook: cancel,
	}
	registerStubs(t, a, b)

	mgr := NewManager(enabledCfg("a", "b"))
	if err := mgr.InstallOne(ctx, "b"); err == nil {
		t.Fatal("expected error from InstallOne; got nil")
	}
	if a.uninstallN.Load() != 1 {
		t.Fatalf("a.uninstallN = %d; want 1 (reverse rollback must run after ctx cancel)", a.uninstallN.Load())
	}
	if !a.uninstallCtxLive.Load() {
		t.Error("rollback Uninstall received a cancelled ctx; want a detached live ctx")
	}
}
