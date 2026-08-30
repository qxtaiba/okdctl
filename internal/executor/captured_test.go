package executor

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRunCaptured(t *testing.T) {
	t.Run("env filtered", func(t *testing.T) {
		t.Setenv("OKDCTL_SECRET_CANARY", "must-not-leak")
		err := RunCaptured(context.Background(), "sh", "-c",
			`[ -z "$OKDCTL_SECRET_CANARY" ] || exit 42`)
		if err != nil {
			t.Fatalf("canary env var leaked into child process: %v", err)
		}
	})

	t.Run("exit one with stderr", func(t *testing.T) {
		err := RunCaptured(context.Background(), "sh", "-c", "echo oops >&2; exit 1")
		if err == nil {
			t.Fatal("exit 1 should return non-nil error")
		}
		if !strings.Contains(err.Error(), "oops") {
			t.Errorf("err = %q; want stderr text 'oops' in message", err.Error())
		}
		var exitErr *ExitError
		if !errors.As(err, &exitErr) {
			t.Errorf("err = %T; want *ExitError unwrappable via errors.As", err)
		}
	})

	t.Run("ctx cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := RunCaptured(ctx, "sh", "-c", "exit 0")
		if err == nil {
			t.Fatal("cancelled ctx should return non-nil error")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v; want errors.Is(_, context.Canceled)", err)
		}
	})
}

func TestOutputCaptured(t *testing.T) {
	out, err := OutputCaptured(context.Background(), "sh", "-c", "printf hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != "hello" {
		t.Errorf("output = %q; want %q", out, "hello")
	}
}
