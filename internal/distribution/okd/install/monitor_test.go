package install

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
)

func installFakeOpenShift(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake binary relies on POSIX sh")
	}
	dir := t.TempDir()
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
	path := filepath.Join(dir, "openshift-install")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

type fakeApprover struct {
	approveN int
	calls    atomic.Int32
}

func (f *fakeApprover) ApprovePendingCSRs(_ context.Context) (int, error) {
	f.calls.Add(1)
	return f.approveN, nil
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

	err := p.MonitorInstallation(context.Background(), t.TempDir(), baseOpts(20*time.Millisecond), approver)
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

// monitorCaptureHandler records slog.Records so synctest tests can assert log
// output produced by MonitorInstallation.
type monitorCaptureHandler struct {
	records []slog.Record
}

func (h *monitorCaptureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *monitorCaptureHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r)
	return nil
}
func (h *monitorCaptureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *monitorCaptureHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *monitorCaptureHandler) hasMessage(msg string) bool {
	for _, r := range h.records {
		if r.Message == msg {
			return true
		}
	}
	return false
}

func newPhaseSynctest(t *testing.T, start func(context.Context, string) (<-chan error, func(), error)) (*Phase, *monitorCaptureHandler) {
	t.Helper()
	h := &monitorCaptureHandler{}
	return &Phase{
		BasePhase:       phase.NewBasePhase("test", phase.WithLogger(slog.New(h))),
		startMonitorCmd: start,
	}, h
}

func TestMonitorInstallation_TickerApproveCSRs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		done := make(chan error, 1)
		p, _ := newPhaseSynctest(t, func(_ context.Context, _ string) (<-chan error, func(), error) {
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
		p, h := newPhaseSynctest(t, func(_ context.Context, _ string) (<-chan error, func(), error) {
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
		time.Sleep(31 * time.Second)
		synctest.Wait()

		<-errc

		if !h.hasMessage("install: process did not exit after kill, abandoning reap") {
			t.Error("expected abandon-reap log message; not found in captured records")
		}
	})
}
