package system

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func alwaysRetryable(error) bool { return true }

func TestRetry_SucceedsFirstAttempt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		err := Retry(context.Background(), DefaultBackoff(), alwaysRetryable, func(context.Context) error {
			calls.Add(1)
			return nil
		})
		if err != nil {
			t.Errorf("err = %v; want nil", err)
		}
		if calls.Load() != 1 {
			t.Errorf("calls = %d; want 1", calls.Load())
		}
	})
}

func TestRetry_SucceedsOnAttemptN(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		err := Retry(context.Background(), DefaultBackoff(), alwaysRetryable, func(context.Context) error {
			n := calls.Add(1)
			if n < 3 {
				return errors.New("not yet")
			}
			return nil
		})
		if err != nil {
			t.Errorf("err = %v; want nil", err)
		}
		if calls.Load() != 3 {
			t.Errorf("calls = %d; want 3", calls.Load())
		}
	})
}

func TestRetry_NonRetryableAbortsImmediately(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		permErr := errors.New("permanent")
		err := Retry(context.Background(), DefaultBackoff(), func(error) bool { return false }, func(context.Context) error {
			calls.Add(1)
			return permErr
		})
		if !errors.Is(err, permErr) {
			t.Errorf("err = %v; want permErr", err)
		}
		if calls.Load() != 1 {
			t.Errorf("calls = %d; want 1 (no retry on non-retryable)", calls.Load())
		}
	})
}

func TestRetry_AllFailuresReturnsLastErr(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		sentinel := errors.New("always fails")
		b := DefaultBackoff()
		err := Retry(context.Background(), b, alwaysRetryable, func(context.Context) error {
			calls.Add(1)
			return sentinel
		})
		if !errors.Is(err, sentinel) {
			t.Errorf("err = %v; want sentinel error %v", err, sentinel)
		}
		if int(calls.Load()) != b.Steps {
			t.Errorf("calls = %d; want %d", calls.Load(), b.Steps)
		}
	})
}

func TestRetry_CtxCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			time.Sleep(DefaultBackoff().Duration / 2)
			cancel()
		}()
		err := Retry(ctx, DefaultBackoff(), alwaysRetryable, func(context.Context) error {
			return errors.New("fail")
		})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v; want context.Canceled", err)
		}
	})
}
