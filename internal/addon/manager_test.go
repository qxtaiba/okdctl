package addon

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/testutil"
)

// installFakeOC satisfies the executor.CommandExists("oc") guard in
// InstallAll/InstallOne.
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
	// installHook, if set, runs during Install (used to cancel ctx mid-install).
	installHook func()
	// uninstallCtxLive records whether the last Uninstall saw a live ctx.
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

// configurableStub also satisfies ConfigurableAddon to exercise the
// pre-install ValidateSettings gate.
type configurableStub struct {
	*stubAddon
	validateErrs []string
}

func (c *configurableStub) DefaultSettings() map[string]string          { return nil }
func (c *configurableStub) ValidateSettings(map[string]string) []string { return c.validateErrs }

func TestInstallOne_SettingsGate(t *testing.T) {
	t.Run("invalid settings blocks install", func(t *testing.T) {
		installFakeOC(t)
		stub := &stubAddon{meta: Metadata{Name: "cfg", DisplayName: "Cfg"}}
		registerStubs(t, &configurableStub{stubAddon: stub, validateErrs: []string{"repository is required"}})

		mgr := NewManager(enabledCfg("cfg"), WithExecutor(executor.New()))
		err := mgr.InstallOne(context.Background(), "cfg")
		if err == nil {
			t.Fatal("expected InstallOne to fail on invalid settings; got nil")
		}
		var cfgErr *errtypes.ConfigError
		if !errors.As(err, &cfgErr) {
			t.Errorf("err = %v; want *errtypes.ConfigError", err)
		}
		if !strings.Contains(err.Error(), "repository is required") {
			t.Errorf("err = %v; want it to surface the validation message", err)
		}
		if stub.installN.Load() != 0 {
			t.Errorf("Install ran %d times; want 0 (settings gate should block it)", stub.installN.Load())
		}
	})

	t.Run("valid settings proceeds", func(t *testing.T) {
		installFakeOC(t)
		stub := &stubAddon{meta: Metadata{Name: "cfgok", DisplayName: "CfgOK"}}
		registerStubs(t, &configurableStub{stubAddon: stub, validateErrs: nil})

		mgr := NewManager(enabledCfg("cfgok"), WithExecutor(executor.New()))
		if err := mgr.InstallOne(context.Background(), "cfgok"); err != nil {
			t.Fatalf("InstallOne with valid settings: %v", err)
		}
		if stub.installN.Load() != 1 {
			t.Errorf("Install ran %d times; want 1", stub.installN.Load())
		}
	})
}

func registerStubs(t *testing.T, stubs ...Addon) {
	t.Helper()
	registry.mu.Lock()
	for _, s := range stubs {
		registry.addons[s.Info().Name] = s
		registry.order = append(registry.order, s.Info().Name)
	}
	registry.mu.Unlock()
	t.Cleanup(func() {
		registry.mu.Lock()
		for _, s := range stubs {
			delete(registry.addons, s.Info().Name)
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

// Rollback must run under a detached ctx even when ctx cancellation caused
// the install failure.
func TestRollbackRunsAfterCtxCancel(t *testing.T) {
	cases := []struct {
		name    string
		enabled []string
		// setup wires the failing install and returns the addon to assert
		// plus the run func.
		setup func(t *testing.T, cancel context.CancelFunc) (*stubAddon, func(*Manager, context.Context) error)
	}{
		{"install_all", []string{"a"}, func(t *testing.T, cancel context.CancelFunc) (*stubAddon, func(*Manager, context.Context) error) {
			a := &stubAddon{
				meta:        Metadata{Name: "a", Priority: 1, DisplayName: "a"},
				installErr:  errors.New("a install fails"),
				installHook: cancel,
			}
			registerStubs(t, a)
			return a, func(m *Manager, ctx context.Context) error { return m.InstallAll(ctx) }
		}},
		{"install_one reverse", []string{"a", "b"}, func(t *testing.T, cancel context.CancelFunc) (*stubAddon, func(*Manager, context.Context) error) {
			a := &stubAddon{meta: Metadata{Name: "a", Priority: 1, DisplayName: "a"}}
			b := &stubAddon{
				meta:        Metadata{Name: "b", Priority: 2, DisplayName: "b", Dependencies: []string{"a"}},
				installErr:  errors.New("b install fails"),
				installHook: cancel,
			}
			registerStubs(t, a, b)
			return a, func(m *Manager, ctx context.Context) error { return m.InstallOne(ctx, "b") }
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			installFakeOC(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			rolledBack, run := tc.setup(t, cancel)

			mgr := NewManager(enabledCfg(tc.enabled...))
			if err := run(mgr, ctx); err == nil {
				t.Fatal("expected error, got nil")
			}
			if rolledBack.uninstallN.Load() != 1 {
				t.Fatalf("uninstallN = %d; want 1 (rollback must run after ctx cancel)", rolledBack.uninstallN.Load())
			}
			if !rolledBack.uninstallCtxLive.Load() {
				t.Error("rollback Uninstall received a cancelled ctx; want a detached live ctx")
			}
		})
	}
}

func TestUninstall_UnknownAddonIsConfigError(t *testing.T) {
	mgr := NewManager(enabledCfg())
	err := mgr.Uninstall(context.Background(), "no-such-addon")
	if err == nil {
		t.Fatal("unknown addon: want error, got nil")
	}
	var cfgErr *errtypes.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Errorf("want *errtypes.ConfigError (exit 2), got %T: %v", err, err)
	}
}

func TestUninstall_RefusedForDependents(t *testing.T) {
	cases := []struct {
		name       string
		withC      bool // also register c (depends on b), forming a transitive chain
		enabled    []string
		wantSubstr string
	}{
		{"direct dependent enabled", false, []string{"a", "b"}, "b depends on it"},
		// Only a and c are enabled: the walk must cross the disabled middle hop.
		{"transitive dependent", true, []string{"a", "c"}, "c depends on it"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := &stubAddon{meta: Metadata{Name: "a", Priority: 1, DisplayName: "a"}}
			b := &stubAddon{meta: Metadata{Name: "b", Priority: 2, DisplayName: "b", Dependencies: []string{"a"}}}
			stubs := []Addon{a, b}
			if tc.withC {
				stubs = append(stubs, &stubAddon{meta: Metadata{Name: "c", Priority: 3, DisplayName: "c", Dependencies: []string{"b"}}})
			}
			registerStubs(t, stubs...)

			mgr := NewManager(enabledCfg(tc.enabled...))
			err := mgr.Uninstall(context.Background(), "a")
			if err == nil {
				t.Fatal("uninstall of a load-bearing addon must be refused")
			}
			var cfgErr *errtypes.ConfigError
			if !errors.As(err, &cfgErr) {
				t.Errorf("want *errtypes.ConfigError, got %T: %v", err, err)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("refusal must name the dependent addon: %v", err)
			}
			if a.uninstallN.Load() != 0 {
				t.Errorf("a.uninstallN = %d; guard must run before Uninstall", a.uninstallN.Load())
			}
		})
	}
}

func TestUninstall_DependencyCycleTerminatesAndProceeds(t *testing.T) {
	x := &stubAddon{meta: Metadata{Name: "x", Priority: 1, DisplayName: "x", Dependencies: []string{"y"}}}
	y := &stubAddon{meta: Metadata{Name: "y", Priority: 2, DisplayName: "y", Dependencies: []string{"x"}}}
	solo := &stubAddon{meta: Metadata{Name: "solo", Priority: 3, DisplayName: "solo"}}
	registerStubs(t, x, y, solo)

	// dependsOn must terminate via the visited map on the x<->y cycle, then proceed.
	mgr := NewManager(enabledCfg("x", "y", "solo"))
	if err := mgr.Uninstall(context.Background(), "solo"); err != nil {
		t.Fatalf("Uninstall(solo): %v", err)
	}
	if solo.uninstallN.Load() != 1 {
		t.Errorf("solo.uninstallN = %d; want exactly 1", solo.uninstallN.Load())
	}
}

func TestUninstall_NoDependentCallsUninstallOnce(t *testing.T) {
	a := &stubAddon{meta: Metadata{Name: "a", Priority: 1, DisplayName: "a"}}
	b := &stubAddon{meta: Metadata{Name: "b", Priority: 2, DisplayName: "b", Dependencies: []string{"a"}}}
	registerStubs(t, a, b)

	// b depends on a, not the reverse — removing the leaf must be allowed.
	mgr := NewManager(enabledCfg("a", "b"))
	if err := mgr.Uninstall(context.Background(), "b"); err != nil {
		t.Fatalf("Uninstall(b): %v", err)
	}
	if b.uninstallN.Load() != 1 {
		t.Errorf("b.uninstallN = %d; want exactly 1", b.uninstallN.Load())
	}
	if a.uninstallN.Load() != 0 {
		t.Errorf("a.uninstallN = %d; dependency must stay installed", a.uninstallN.Load())
	}
}

func TestVerifyAll_ContinuesPastFailureAndAggregates(t *testing.T) {
	a := &stubAddon{meta: Metadata{Name: "a", Priority: 1, DisplayName: "a"}, verifyErr: errors.New("a broken")}
	b := &stubAddon{meta: Metadata{Name: "b", Priority: 2, DisplayName: "b"}}
	registerStubs(t, a, b)

	mgr := NewManager(enabledCfg("a", "b"))
	results, err := mgr.VerifyAll(context.Background())
	if err == nil {
		t.Fatal("want aggregated error when a verify fails")
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d; want 2 (iteration must not stop on failure)", len(results))
	}
	var clusterErr *errtypes.ClusterError
	if !errors.As(err, &clusterErr) {
		t.Errorf("aggregate must carry *errtypes.ClusterError, got %T: %v", err, err)
	}
	byName := map[string]error{}
	for _, r := range results {
		byName[r.Name] = r.Err
	}
	if byName["a"] == nil {
		t.Error("result for a must carry the verify error")
	}
	if byName["b"] != nil {
		t.Errorf("result for b must be clean, got: %v", byName["b"])
	}
	if b.verifyN.Load() != 1 {
		t.Errorf("b.verifyN = %d; want 1", b.verifyN.Load())
	}
}

func TestVerifyAll_CancelledCtxStopsIteration(t *testing.T) {
	a := &stubAddon{meta: Metadata{Name: "a", Priority: 1, DisplayName: "a"}}
	registerStubs(t, a)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewManager(enabledCfg("a")).VerifyAll(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got: %v", err)
	}
	if a.verifyN.Load() != 0 {
		t.Errorf("a.verifyN = %d; cancelled ctx must stop before Verify", a.verifyN.Load())
	}
}
