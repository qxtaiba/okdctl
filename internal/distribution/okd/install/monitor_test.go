package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

func installFakeOpenShift(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake binary relies on POSIX sh")
	}
	dir := t.TempDir()
	script := `#!/bin/sh
case "${OC_FAKE_MODE:-ok}" in
  ok)
    exit 0
    ;;
  sleep)
    sleep 300
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
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	err := p.MonitorInstallation(ctx, t.TempDir(), baseOpts(30*time.Second), approver)
	if err == nil {
		t.Fatal("expected error on cancellation; got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v; want errors.Is(_, context.Canceled)", err)
	}
}
