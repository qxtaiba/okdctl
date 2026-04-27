package system

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

// Guards the env-allowlist forwarding contract for RunCaptured; the wrapper
// must not pass arbitrary parent env to privileged subprocesses.
func TestRunCaptured_EnvFiltered(t *testing.T) {
	t.Setenv("OKDCTL_SECRET_CANARY", "must-not-leak")
	err := RunCaptured(context.Background(), "sh", "-c",
		`[ -z "$OKDCTL_SECRET_CANARY" ] || exit 42`)
	if err != nil {
		t.Fatalf("canary env var leaked into child process: %v", err)
	}
}

func TestWaitFor_ReadyOnFirstCheck(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		err := WaitFor(ctx, "test", "initial check", func() bool { return true }, WaitForOptions{
			Interval: time.Second,
			Timeout:  5 * time.Second,
			Logger:   logutil.NopLogger,
		})
		if err != nil {
			t.Errorf("immediate ready should be nil; got %v", err)
		}
	})
}

func TestWaitFor_ReadyAfterTicks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		check := func() bool {
			calls++
			return calls >= 3
		}

		err := WaitFor(context.Background(), "test", "delayed", check, WaitForOptions{
			Interval: 2 * time.Second,
			Timeout:  30 * time.Second,
			Logger:   logutil.NopLogger,
		})
		if err != nil {
			t.Errorf("expected success after 3 calls; got %v", err)
		}
		if calls != 3 {
			t.Errorf("calls = %d; want 3", calls)
		}
	})
}

func TestWaitFor_Timeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		err := WaitFor(context.Background(), "test", "never-ready", func() bool { return false }, WaitForOptions{
			Interval: 1 * time.Second,
			Timeout:  5 * time.Second,
			Logger:   logutil.NopLogger,
		})
		if err == nil {
			t.Fatal("expected timeout error")
		}
		if !strings.Contains(err.Error(), "timeout") {
			t.Errorf("err = %q; want 'timeout' prefix", err.Error())
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("err = %v; want errors.Is(_, context.DeadlineExceeded)", err)
		}
		var clusterErr *errtypes.ClusterError
		if !errors.As(err, &clusterErr) {
			t.Errorf("err = %v; want errors.As(_, *errtypes.ClusterError)", err)
		}
	})
}

// TestWaitFor_CtxCancellation locks that ctx.Err takes priority in the
// message when both ctx and timeout fire.
func TestWaitFor_CtxCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := WaitFor(ctx, "test", "cancelled", func() bool { return false }, WaitForOptions{
			Interval: 1 * time.Second,
			Timeout:  5 * time.Second,
			Logger:   logutil.NopLogger,
		})
		if err == nil {
			t.Fatal("expected ctx error")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v; want wrapping context.Canceled", err)
		}
	})
}

func TestWaitFor_DefaultInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		err := WaitFor(context.Background(), "test", "default-interval", func() bool { return true }, WaitForOptions{
			Interval: 0,
			Timeout:  1 * time.Minute,
			Logger:   logutil.NopLogger,
		})
		if err != nil {
			t.Errorf("default interval path errored: %v", err)
		}
	})
}

func TestWaitForWithTimeout_Convenience(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		err := WaitForWithTimeout(context.Background(), "test", "convenience", func() bool { return true }, 5*time.Second, logutil.NopLogger)
		if err != nil {
			t.Errorf("err = %v", err)
		}
	})
}
