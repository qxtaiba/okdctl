package system

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

// TestWaitFor_ReadyOnFirstCheck locks that the fast-path (ready before the
// first tick) returns immediately with nil.
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

// TestWaitFor_ReadyAfterTicks locks that the ticker loop polls until check
// returns true, within the timeout budget.
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

// TestWaitFor_Timeout locks that a never-ready predicate errors with a
// timeout that errors.Is matches context.DeadlineExceeded.
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
	})
}

// TestWaitFor_CtxCancellation locks that ctx.Err takes priority in the
// message when both ctx and timeout fire.
func TestWaitFor_CtxCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		// Cancel immediately so the first tick after the ctx-checked branch
		// returns ctx.Err, not "ready".
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

// TestWaitFor_DefaultInterval locks that a zero Interval falls back to 30s
// (covered by the default code path) and does not infinite-loop.
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
