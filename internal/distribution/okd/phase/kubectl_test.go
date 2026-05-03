package phase

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

// installFakeOC writes a POSIX shell script named "oc" in a temp dir,
// then prepends that dir to PATH so the Executor's exec.CommandContext
// picks it up. The script switches behaviour on the OC_FAKE_MODE env.
func installFakeOC(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-oc script relies on POSIX sh")
	}
	dir := t.TempDir()
	script := `#!/bin/sh
# Fake oc for testing — behaviour keyed off OC_FAKE_MODE.
case "${OC_FAKE_MODE:-empty}" in
  exists)
    echo "pod/foo Running"
    exit 0
    ;;
  empty)
    exit 0
    ;;
  error)
    echo "cluster unreachable" >&2
    exit 1
    ;;
  ticker)
    # Prints a value that flips past a threshold based on OC_CALL_FILE.
    f="${OC_CALL_FILE:-/tmp/okd-fake-oc-counter}"
    n=$(cat "$f" 2>/dev/null || echo 0)
    n=$((n + 1))
    echo "$n" > "$f"
    if [ "$n" -ge "${OC_READY_AT:-3}" ]; then
      echo "ready"
    else
      echo "waiting"
    fi
    exit 0
    ;;
esac
exit 0
`
	path := filepath.Join(dir, "oc")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func newTestPhase(t *testing.T) *BasePhase {
	t.Helper()
	p := NewBasePhase("test",
		WithExecutor(executor.New()),
		WithLogger(logutil.NopLogger),
	)
	return &p
}

func TestOcResourceExists(t *testing.T) {
	installFakeOC(t)
	p := newTestPhase(t)

	t.Run("non-empty stdout + exit 0 → true", func(t *testing.T) {
		t.Setenv("OC_FAKE_MODE", "exists")
		ok, err := p.OcResourceExists(context.Background(), "check pods", "pods")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !ok {
			t.Errorf("expected true for non-empty stdout")
		}
	})

	t.Run("empty stdout + exit 0 → false", func(t *testing.T) {
		t.Setenv("OC_FAKE_MODE", "empty")
		ok, err := p.OcResourceExists(context.Background(), "check pods", "pods")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if ok {
			t.Errorf("expected false for empty stdout")
		}
	})

	t.Run("exit non-zero → false without error", func(t *testing.T) {
		t.Setenv("OC_FAKE_MODE", "error")
		// oc error prints to stderr and exits 1 — Executor surfaces it as
		// a non-zero ExitCode (not a transport err), so OcResourceExists
		// returns (false, nil) by contract: "returns true only when exit
		// 0 AND stdout non-empty".
		ok, err := p.OcResourceExists(context.Background(), "check", "pods")
		if err != nil {
			t.Errorf("non-zero exit is not a transport error; err = %v", err)
		}
		if ok {
			t.Errorf("expected false on non-zero exit")
		}
	})
}

func TestOcPollOutput(t *testing.T) {
	installFakeOC(t)
	p := newTestPhase(t)

	t.Run("ready-after-N-polls returns captured value", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			dir := t.TempDir()
			counter := filepath.Join(dir, "counter")
			t.Setenv("OC_FAKE_MODE", "ticker")
			t.Setenv("OC_CALL_FILE", counter)
			t.Setenv("OC_READY_AT", "2")

			got, err := p.OcPollOutputInterval(context.Background(), "test", "ready check", 30*time.Second, 50*time.Millisecond, func(s string) bool {
				return s == "ready"
			}, "get", "deploy", "-o", "jsonpath={.status.availableReplicas}")
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != "ready" {
				t.Errorf("captured value = %q; want ready", got)
			}
		})
	})

	t.Run("predicate never true → timeout", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			dir := t.TempDir()
			counter := filepath.Join(dir, "counter")
			t.Setenv("OC_FAKE_MODE", "ticker")
			t.Setenv("OC_CALL_FILE", counter)
			t.Setenv("OC_READY_AT", "9999")

			_, err := p.OcPollOutputInterval(context.Background(), "test", "never", 500*time.Millisecond, 50*time.Millisecond, func(s string) bool {
				return s == "will-not-happen"
			}, "get", "deploy")
			if err == nil {
				t.Fatal("expected timeout error")
			}
			if !strings.Contains(err.Error(), "timeout") && !strings.Contains(err.Error(), "waiting") {
				t.Errorf("err = %q; want timeout/waiting phrasing", err.Error())
			}
		})
	})

	t.Run("ctx cancellation returns ctx error", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			t.Setenv("OC_FAKE_MODE", "empty")
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			_, err := p.OcPollOutputInterval(ctx, "test", "cancelled", 5*time.Second, 50*time.Millisecond, func(string) bool { return false }, "get")
			if err == nil {
				t.Fatal("expected ctx error")
			}
		})
	})
}

func TestOcOutput(t *testing.T) {
	installFakeOC(t)
	p := newTestPhase(t)

	t.Run("exit 0 returns trimmed stdout", func(t *testing.T) {
		t.Setenv("OC_FAKE_MODE", "exists")
		out, err := p.OcOutput(context.Background(), "get", "pods")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if out != "pod/foo Running" {
			t.Errorf("stdout = %q; want %q", out, "pod/foo Running")
		}
	})

	t.Run("exit 1 returns typed ExitError", func(t *testing.T) {
		t.Setenv("OC_FAKE_MODE", "error")
		_, err := p.OcOutput(context.Background(), "get", "pods")
		if err == nil {
			t.Fatal("expected error on exit 1")
		}
		var ee *executor.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("err is %T; want *executor.ExitError", err)
		}
		if ee.ExitCode != 1 {
			t.Errorf("ExitCode = %d; want 1", ee.ExitCode)
		}
		if !strings.Contains(ee.Stderr, "cluster unreachable") {
			t.Errorf("Stderr = %q; want to contain %q", ee.Stderr, "cluster unreachable")
		}
	})

	t.Run("ctx cancel propagates context error", func(t *testing.T) {
		t.Setenv("OC_FAKE_MODE", "exists")
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := p.OcOutput(ctx, "get", "pods")
		if err == nil {
			t.Fatal("expected error on cancelled ctx")
		}
		var ee *executor.ExitError
		if errors.As(err, &ee) {
			t.Errorf("cancelled ctx produced ExitError; want a context error")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v; want context.Canceled in chain", err)
		}
	})
}
