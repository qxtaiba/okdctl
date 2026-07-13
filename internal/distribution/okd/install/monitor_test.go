package install

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/testutil"
)

func installFakeOpenShift(t *testing.T) {
	t.Helper()
	// exec sleep so the shell process is replaced — SIGKILL on the script
	// kills the sleep directly, preventing an orphaned child from holding
	// stdout/stderr pipes open and stalling `go test` shutdown.
	script := `#!/bin/sh
case "${OC_FAKE_MODE:-ok}" in
  ok)
    exit 0
    ;;
  sleep)
    exec sleep 300 < /dev/null > /dev/null 2>&1
    ;;
esac
exit 0
`
	testutil.InstallFakeBin(t, "openshift-install", script)
}

type fakeApprover struct {
	approveN int
	calls    atomic.Int32
}

func (f *fakeApprover) ApprovePendingCSRs(_ context.Context) (int, error) {
	f.calls.Add(1)
	return f.approveN, nil
}

type fakeOperatorCounter struct {
	available, total int
	err              error
}

func (f fakeOperatorCounter) ClusterOperatorsAvailable(context.Context) (available, total int, err error) {
	return f.available, f.total, f.err
}

// TestOperatorStatusDetail locks the status-line detail format across the
// counter states: a live count renders "cluster operators A/B available · N
// CSRs approved", while a nil counter, a count error, or a zero total degrade
// to the CSR count alone rather than blanking the line.
func TestOperatorStatusDetail(t *testing.T) {
	tests := []struct {
		name    string
		counter operatorCounter
		csrs    int
		want    string
	}{
		{"present total>0", fakeOperatorCounter{available: 30, total: 33}, 4, "cluster operators 30/33 available · 4 CSRs approved"},
		{"present all available", fakeOperatorCounter{available: 33, total: 33}, 0, "cluster operators 33/33 available · 0 CSRs approved"},
		{"present count error", fakeOperatorCounter{err: errors.New("api not reachable")}, 5, "5 CSRs approved"},
		{"present zero total", fakeOperatorCounter{available: 0, total: 0}, 2, "2 CSRs approved"},
		{"nil counter", nil, 7, "7 CSRs approved"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := operatorStatusDetail(context.Background(), tc.counter, tc.csrs); got != tc.want {
				t.Errorf("operatorStatusDetail = %q, want %q", got, tc.want)
			}
		})
	}
}

func baseOpts(installTimeout time.Duration) *Options {
	return &Options{
		InstallTimeout:      installTimeout,
		CSRApprovalInterval: 10 * time.Millisecond,
	}
}

func TestMonitorInstallation_Success(t *testing.T) {
	installFakeOpenShift(t)
	t.Setenv("OC_FAKE_MODE", "ok")

	p := newInstallPhase(t)
	approver := &fakeApprover{approveN: 3}

	err := p.MonitorInstallation(context.Background(), t.TempDir(), baseOpts(30*time.Second), approver)
	if err != nil {
		t.Fatalf("expected nil error on success; got %v", err)
	}
	if approver.calls.Load() < 1 {
		t.Errorf("ApprovePendingCSRs call count = %d; want >= 1", approver.calls.Load())
	}
}

func TestMonitorInstallation_DeadlineExceeded(t *testing.T) {
	installFakeOpenShift(t)
	t.Setenv("OC_FAKE_MODE", "sleep")

	p := newInstallPhase(t)
	approver := &fakeApprover{}

	clusterDir := t.TempDir()
	err := p.MonitorInstallation(context.Background(), clusterDir, baseOpts(20*time.Millisecond), approver)
	if err == nil {
		t.Fatal("expected error on timeout; got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v; want errors.Is(_, context.DeadlineExceeded)", err)
	}
	var clusterErr *errtypes.ClusterError
	if !errors.As(err, &clusterErr) {
		t.Errorf("err = %v; want errors.As(_, *errtypes.ClusterError)", err)
	}
	assertTimeoutDiagnostics(t, err, clusterDir)
}

// assertTimeoutDiagnostics pins the enriched-timeout contract: the message
// must name the openshift-install log, an oc probe, and okdctl debug-bundle.
func assertTimeoutDiagnostics(t *testing.T, err error, clusterDir string) {
	t.Helper()
	for _, want := range []string{
		filepath.Join(clusterDir, ".openshift_install.log"),
		"oc --kubeconfig " + filepath.Join(clusterDir, "auth", "kubeconfig") + " get clusteroperators",
		"okdctl debug-bundle",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("timeout error missing %q: %v", want, err)
		}
	}
}

func TestWaitForBootstrap_TimeoutNamesDiagnostics(t *testing.T) {
	installFakeOpenShift(t)
	t.Setenv("OC_FAKE_MODE", "sleep")

	p := newInstallPhase(t)
	clusterDir := t.TempDir()
	opts := &Options{BootstrapTimeout: 20 * time.Millisecond}

	err := p.WaitForBootstrap(context.Background(), clusterDir, opts)
	if err == nil {
		t.Fatal("expected error on timeout; got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v; want errors.Is(_, context.DeadlineExceeded)", err)
	}
	if !strings.Contains(err.Error(), "bootstrap timed out after") {
		t.Errorf("err = %v; want message to name the bootstrap timeout", err)
	}
	assertTimeoutDiagnostics(t, err, clusterDir)
}

func TestMonitorInstallation_CtxCanceled(t *testing.T) {
	installFakeOpenShift(t)
	t.Setenv("OC_FAKE_MODE", "sleep")

	p := newInstallPhase(t)
	approver := &fakeApprover{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := p.MonitorInstallation(ctx, t.TempDir(), baseOpts(30*time.Second), approver)
	if err == nil {
		t.Fatal("expected error on cancellation; got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v; want errors.Is(_, context.Canceled)", err)
	}
}

func newPhaseSynctest(t *testing.T, start func(context.Context, string) (<-chan error, func(), error)) *Phase {
	t.Helper()
	return &Phase{
		BasePhase:       phase.NewBasePhase(phase.WithLogger(logutil.NopLogger), phase.WithReporter(logutil.NopProgressReporter)),
		startMonitorCmd: start,
	}
}

func TestMonitorInstallation_TickerApproveCSRs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		done := make(chan error, 1)
		p := newPhaseSynctest(t, func(_ context.Context, _ string) (<-chan error, func(), error) {
			return done, func() {}, nil
		})
		approver := &fakeApprover{approveN: 1}
		opts := &Options{
			InstallTimeout:      5 * time.Minute,
			CSRApprovalInterval: 1 * time.Second,
		}

		ctx, cancel := context.WithCancel(context.Background())
		errc := make(chan error, 1)
		go func() {
			errc <- p.MonitorInstallation(ctx, t.TempDir(), opts, approver)
		}()

		synctest.Wait()
		time.Sleep(2 * time.Second)
		synctest.Wait()

		cancel()
		synctest.Wait()
		close(done)
		<-errc

		if approver.calls.Load() < 1 {
			t.Errorf("ApprovePendingCSRs calls = %d; want >= 1", approver.calls.Load())
		}
	})
}

func TestMonitorInstallation_ReapTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		done := make(chan error, 1)
		p := newPhaseSynctest(t, func(_ context.Context, _ string) (<-chan error, func(), error) {
			return done, func() {}, nil
		})
		approver := &fakeApprover{}
		opts := &Options{
			InstallTimeout:      5 * time.Minute,
			CSRApprovalInterval: 1 * time.Minute,
		}

		ctx, cancel := context.WithCancel(context.Background())
		errc := make(chan error, 1)
		go func() {
			errc <- p.MonitorInstallation(ctx, t.TempDir(), opts, approver)
		}()

		synctest.Wait()
		cancel()
		synctest.Wait()

		err := <-errc
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v; want context.Canceled", err)
		}
	})
}

func TestMonitorInstallation_CtxCancelReapsGracefully(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		done := make(chan error, 1)
		p := newPhaseSynctest(t, func(_ context.Context, _ string) (<-chan error, func(), error) {
			return done, func() {}, nil
		})
		approver := &fakeApprover{}
		opts := &Options{
			InstallTimeout:      5 * time.Minute,
			CSRApprovalInterval: 1 * time.Minute,
		}

		ctx, cancel := context.WithCancel(context.Background())
		errc := make(chan error, 1)
		go func() {
			errc <- p.MonitorInstallation(ctx, t.TempDir(), opts, approver)
		}()

		synctest.Wait()
		cancel()
		synctest.Wait()

		err := <-errc
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v; want context.Canceled", err)
		}
	})
}
