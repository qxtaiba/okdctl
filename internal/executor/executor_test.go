package executor

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

func TestWithCancelSignal(t *testing.T) {
	t.Run("defaults to SIGTERM", func(t *testing.T) {
		e := New()
		if e.cancelSignal != syscall.SIGTERM {
			t.Errorf("cancelSignal = %v; want SIGTERM", e.cancelSignal)
		}
	})

	t.Run("WithCancelSignal overrides", func(t *testing.T) {
		e := New(WithCancelSignal(syscall.SIGINT))
		if e.cancelSignal != syscall.SIGINT {
			t.Errorf("cancelSignal = %v; want SIGINT", e.cancelSignal)
		}
	})
}

func TestRunDiscard(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs POSIX sh")
	}

	t.Run("stdout is always empty", func(t *testing.T) {
		t.Parallel()
		e := New(WithInheritedEnv())
		result, err := e.RunDiscard(context.Background(), "sh", "-c", "printf 'noise\n'; exit 0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Stdout != "" {
			t.Errorf("Stdout = %q; want empty (discarded)", result.Stdout)
		}
	})

	t.Run("non-zero exit returns ExitError via RunDiscardChecked", func(t *testing.T) {
		t.Parallel()
		e := New(WithInheritedEnv())
		_, err := e.RunDiscardChecked(context.Background(), "sh", "-c", "echo oops >&2; exit 3")
		if err == nil {
			t.Fatal("expected error for exit 3")
		}
		var exitErr *ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("err type = %T; want *ExitError", err)
		}
		if exitErr.ExitCode != 3 {
			t.Errorf("ExitCode = %d; want 3", exitErr.ExitCode)
		}
		if !strings.Contains(exitErr.Stderr, "oops") {
			t.Errorf("Stderr = %q; want it to contain 'oops'", exitErr.Stderr)
		}
	})
}

func TestBuildEnv_AllowlistFilters(t *testing.T) {
	// Seed some env vars — allowed and disallowed.
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("HOME", "/tmp/home")
	t.Setenv("KUBECONFIG", "/tmp/kube")
	t.Setenv("KUBE_PS1", "on")                // prefix match
	t.Setenv("TF_VAR_region", "us-east-1")    // prefix match
	t.Setenv("PROXMOX_VE_PASSWORD", "hunter") // prefix match
	t.Setenv("AWS_SECRET_ACCESS_KEY", "nope") // not in allowlist
	t.Setenv("SECRET_API_KEY", "nope")        // not in allowlist
	t.Setenv("OKDCTL_INTERNAL_TOKEN", "nope") // not in allowlist

	e := New()
	env := e.buildEnv()
	joined := "\x00" + strings.Join(env, "\x00") + "\x00"

	mustContain := []string{
		"PATH=/usr/bin:/bin",
		"HOME=/tmp/home",
		"KUBECONFIG=/tmp/kube",
		"KUBE_PS1=on",
		"TF_VAR_region=us-east-1",
		"PROXMOX_VE_PASSWORD=hunter",
	}
	for _, want := range mustContain {
		if !strings.Contains(joined, "\x00"+want+"\x00") {
			t.Errorf("expected %q in env; got env:\n%v", want, env)
		}
	}

	mustNotContain := []string{
		"AWS_SECRET_ACCESS_KEY=",
		"SECRET_API_KEY=",
		"OKDCTL_INTERNAL_TOKEN=",
	}
	for _, forbid := range mustNotContain {
		if strings.Contains(joined, "\x00"+forbid) {
			t.Errorf("%q leaked through allowlist; env:\n%v", forbid, env)
		}
	}
}

func TestBuildEnv_WithEnvAppendsAfterAllowlist(t *testing.T) {
	t.Setenv("KUBECONFIG", "/parent/kube")
	e := New(WithEnv([]string{"KUBECONFIG=/caller/kube", "CUSTOM_VAR=value"}))

	env := e.buildEnv()
	// Caller override should appear AFTER the allowlist-filtered parent,
	// so os/exec uses the last occurrence (Go's contract).
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "KUBECONFIG=/caller/kube") {
		t.Errorf("caller KUBECONFIG missing: %v", env)
	}
	if !strings.Contains(joined, "CUSTOM_VAR=value") {
		t.Errorf("caller CUSTOM_VAR missing: %v", env)
	}
	// Verify order: caller override comes AFTER the parent entry.
	parentIdx := strings.Index(joined, "KUBECONFIG=/parent/kube")
	callerIdx := strings.Index(joined, "KUBECONFIG=/caller/kube")
	if parentIdx < 0 || callerIdx < 0 || callerIdx < parentIdx {
		t.Errorf("order wrong — parent=%d caller=%d, want caller > parent",
			parentIdx, callerIdx)
	}
}

func TestBuildEnv_InheritedNilWhenNoCallerEnv(t *testing.T) {
	// cmd.Env = nil is the os/exec contract for "inherit os.Environ() verbatim".
	// We return nil so the subprocess gets the full parent environment
	// without us having to enumerate it.
	e := New(WithInheritedEnv())
	env := e.buildEnv()
	if env != nil {
		t.Errorf("expected nil env (os/exec contract for full inheritance); got %v", env)
	}
}

func TestBuildEnv_InheritedWithAppendedVars(t *testing.T) {
	t.Setenv("OKDCTL_INTERNAL_TOKEN", "inherited")
	// When WithInheritedEnv is combined with WithEnv, we have to materialize
	// os.Environ() so we can append to it — nil would drop the caller's vars.
	e := New(WithInheritedEnv(), WithEnv([]string{"EXTRA=1"}))
	env := e.buildEnv()
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "OKDCTL_INTERNAL_TOKEN=inherited") {
		t.Errorf("inherited env missed non-allowlist var: %v", env)
	}
	if !strings.Contains(joined, "EXTRA=1") {
		t.Errorf("caller env not appended: %v", env)
	}
}

func TestAllowlist_ExactAndPrefix(t *testing.T) {
	cases := []struct {
		key  string
		want bool
	}{
		{"PATH", true},
		{"HOME", true},
		{"KUBECONFIG", true},
		{"KUBE_PS1", true},
		{"KUBERNETES_SERVICE_HOST", true},
		{"TF_VAR_thing", true},
		{"TF_LOG", true},
		{"PROXMOX_VE_API_TOKEN", true},
		{"OC_EDITOR", true},
		{"HELM_CACHE_HOME", true},
		{"GIT_SSH_COMMAND", true},
		{"GIT_TERMINAL_PROMPT", true},
		{"GITHUB_TOKEN", false},
		{"GH_TOKEN", false},
		{"AWS_SECRET_ACCESS_KEY", false},
		{"DOCKER_PASSWORD", false},
		{"GPG_PRIVATE_KEY", false},
		{"", false},
	}
	for _, tc := range cases {
		got := DefaultEnvAllowlist.allows(tc.key)
		if got != tc.want {
			t.Errorf("allows(%q) = %v; want %v", tc.key, got, tc.want)
		}
	}
}

// TestBuildEnv_EndToEndWithEcho verifies the subprocess actually sees the
// filtered env by echoing it via the shell. Skipped on Windows (no `env`).
func TestBuildEnv_EndToEndWithEcho(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs POSIX env")
	}
	// An obvious leak candidate the allowlist rejects.
	t.Setenv("OKDCTL_SECRET_PROBE", "xyz123")

	e := New()
	result, err := e.Run(context.Background(), "env")
	if err != nil {
		t.Fatalf("env: %v", err)
	}
	if strings.Contains(result.Stdout, "OKDCTL_SECRET_PROBE=xyz123") {
		t.Errorf("secret leaked into subprocess env:\n%s", result.Stdout)
	}
	if !strings.Contains(result.Stdout, "PATH=") {
		t.Errorf("PATH missing from subprocess env:\n%s", result.Stdout)
	}
	_ = os.Unsetenv("OKDCTL_SECRET_PROBE")
}

func TestZeroizeEnv(t *testing.T) {
	t.Run("blanks cred entries and nils slice", func(t *testing.T) {
		e := New(WithEnv([]string{
			"PROXMOX_VE_PASSWORD=secret",
			"PROXMOX_VE_API_TOKEN=tok123",
			"KUBECONFIG=/etc/kube",
		}))
		e.ZeroizeEnv()
		if e.env != nil {
			t.Errorf("Env not nil after ZeroizeEnv; got %v", e.env)
		}
	})

	t.Run("non-cred entries are blanked by clear before nil", func(t *testing.T) {
		e := New(WithEnv([]string{
			"PROXMOX_VE_PASSWORD=hunter2",
			"KUBECONFIG=/etc/kube",
		}))
		snap := e.env
		e.ZeroizeEnv()
		if snap[0] != "" {
			t.Errorf("cred entry not blanked before clear; got %q", snap[0])
		}
		if snap[1] != "" {
			t.Errorf("non-cred entry not zeroed by clear; got %q", snap[1])
		}
	})

	t.Run("nil and empty Env are no-ops", func(_ *testing.T) {
		e1 := New()
		e1.ZeroizeEnv()

		e2 := New(WithEnv([]string{}))
		e2.ZeroizeEnv()
	})
}

func TestSnapshotEnv(t *testing.T) {
	t.Run("copy is independent of internal env", func(t *testing.T) {
		e := New(WithEnv([]string{"KEY=v1"}))
		s := e.SnapshotEnv()
		s[0] = "KEY=v2"
		if e.env[0] != "KEY=v1" {
			t.Errorf("mutation of snapshot altered internal env: got %q", e.env[0])
		}
	})

	t.Run("length matches internal env", func(t *testing.T) {
		kvs := []string{"A=1", "B=2", "C=3"}
		e := New(WithEnv(kvs))
		s := e.SnapshotEnv()
		if len(s) != len(e.env) {
			t.Errorf("len(snapshot)=%d; want %d", len(s), len(e.env))
		}
	})

	t.Run("empty env returns non-nil zero-length slice", func(t *testing.T) {
		e := New(WithEnv([]string{}))
		s := e.SnapshotEnv()
		if s == nil {
			t.Error("snapshot of empty env must be non-nil")
		}
		if len(s) != 0 {
			t.Errorf("len(snapshot)=%d; want 0", len(s))
		}
	})
}

func TestExitError_ErrorTruncatesLongStderr(t *testing.T) {
	long := strings.Repeat("x", 500)
	e := &ExitError{Command: "terraform apply", ExitCode: 1, Stderr: long}
	got := e.Error()
	if !strings.Contains(got, "[truncated]") {
		t.Errorf("Error() missing truncation marker for 500-byte stderr; got: %q", got)
	}
	if strings.Contains(got, long) {
		t.Errorf("Error() embedded full 500-byte stderr verbatim")
	}
}

func TestExitError_ErrorShortStderrPassesThrough(t *testing.T) {
	e := &ExitError{Command: "terraform plan", ExitCode: 2, Stderr: "permission denied"}
	got := e.Error()
	if !strings.Contains(got, "permission denied") {
		t.Errorf("Error() dropped short stderr; got: %q", got)
	}
	if strings.Contains(got, "[truncated]") {
		t.Errorf("Error() added truncation marker for short stderr; got: %q", got)
	}
}

func TestRunStreamedChecked(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs POSIX sh")
	}

	t.Run("zero exit streams stdout and returns no error", func(t *testing.T) {
		t.Parallel()
		var out, errBuf strings.Builder
		e := New(WithInheritedEnv(), WithStdout(&out), WithStderr(&errBuf))

		result, err := e.RunStreamedChecked(context.Background(), "sh", "-c", "printf 'hello\nworld\n'")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.ExitCode != 0 {
			t.Errorf("ExitCode = %d; want 0", result.ExitCode)
		}
		if !strings.Contains(out.String(), "hello") {
			t.Errorf("e.Stdout missing streamed output; got %q", out.String())
		}
		if !strings.Contains(result.Stdout, "hello") {
			t.Errorf("result.Stdout missing captured output; got %q", result.Stdout)
		}
	})

	t.Run("non-zero exit returns ExitError with stderr tail", func(t *testing.T) {
		t.Parallel()
		var out, errBuf strings.Builder
		e := New(WithInheritedEnv(), WithStdout(&out), WithStderr(&errBuf))

		result, err := e.RunStreamedChecked(context.Background(), "sh", "-c",
			"printf 'boom\n' >&2; exit 1")
		if err == nil {
			t.Fatal("expected error for exit 1; got nil")
		}
		var exitErr *ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("err type = %T; want *ExitError", err)
		}
		if exitErr.ExitCode != 1 {
			t.Errorf("ExitError.ExitCode = %d; want 1", exitErr.ExitCode)
		}
		if !strings.Contains(exitErr.Stderr, "boom") {
			t.Errorf("ExitError.Stderr = %q; want it to contain 'boom'", exitErr.Stderr)
		}
		if result == nil {
			t.Fatal("result must be non-nil on error")
		}
		if !strings.Contains(errBuf.String(), "boom") {
			t.Errorf("e.Stderr missing streamed output; got %q", errBuf.String())
		}
	})

	t.Run("ctx cancel returns context error", func(t *testing.T) {
		t.Parallel()
		e := New(WithInheritedEnv(), WithStdout(&strings.Builder{}), WithStderr(&strings.Builder{}))

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := e.RunStreamedChecked(ctx, "sh", "-c", "sleep 10")
		if err == nil {
			t.Fatal("expected error after ctx cancel; got nil")
		}
		var exitErr *ExitError
		if errors.As(err, &exitErr) {
			t.Errorf("got *ExitError on ctx cancel; want context error, got: %v", err)
		}
	})
}

func TestRunInteractive_CtxCancelReturnsCtxErr(t *testing.T) {
	t.Parallel()

	e := New(WithInheritedEnv(), WithStdout(&strings.Builder{}), WithStderr(&strings.Builder{}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := e.RunInteractive(ctx, "sh", "-c", "sleep 10")
	if err == nil {
		t.Fatal("expected error after ctx cancel; got nil")
	}
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		t.Errorf("got *ExitError on ctx cancel; want context error, got: %v", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v; want errors.Is(err, context.Canceled) == true", err)
	}
}

func TestRunOutput_FullCapture(t *testing.T) {
	t.Run("captures more than constMaxLines lines", func(t *testing.T) {
		t.Parallel()
		e := New(WithInheritedEnv())
		result, err := e.RunOutput(context.Background(), 0, "sh", "-c",
			"for i in $(seq 1 300); do echo \"line$i\"; done")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.ExitCode != 0 {
			t.Errorf("ExitCode = %d; want 0", result.ExitCode)
		}
		if result.Truncated {
			t.Errorf("Truncated = true for 300 lines under 4 MiB cap; want false")
		}
		if !strings.Contains(result.Stdout, "line1\n") {
			t.Errorf("stdout missing line1 (head)")
		}
		if !strings.Contains(result.Stdout, "line300") {
			t.Errorf("stdout missing line300 (tail)")
		}
	})

	t.Run("truncates at byte cap and sets Truncated", func(t *testing.T) {
		t.Parallel()
		e := New(WithInheritedEnv())
		result, err := e.RunOutput(context.Background(), 10, "sh", "-c",
			"printf '%0.s0123456789' $(seq 1 10)")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Truncated {
			t.Errorf("Truncated = false; want true for output > 10 bytes")
		}
		if len(result.Stdout) != 10 {
			t.Errorf("len(Stdout) = %d; want 10", len(result.Stdout))
		}
	})

	t.Run("non-zero exit returns ExitError", func(t *testing.T) {
		t.Parallel()
		e := New(WithInheritedEnv())
		result, err := e.RunOutputChecked(context.Background(), 0, "sh", "-c",
			"printf 'out\n'; printf 'err\n' >&2; exit 2")
		if err == nil {
			t.Fatal("expected error for exit 2; got nil")
		}
		var exitErr *ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("err type = %T; want *ExitError", err)
		}
		if exitErr.ExitCode != 2 {
			t.Errorf("ExitCode = %d; want 2", exitErr.ExitCode)
		}
		if result == nil {
			t.Fatal("result must be non-nil on error")
		}
		if !strings.Contains(result.Stdout, "out") {
			t.Errorf("result.Stdout missing captured output; got %q", result.Stdout)
		}
	})

	t.Run("zero exit returns full stdout", func(t *testing.T) {
		t.Parallel()
		e := New(WithInheritedEnv())
		result, err := e.RunOutputChecked(context.Background(), 0, "sh", "-c", "printf 'hello'")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Stdout != "hello" {
			t.Errorf("Stdout = %q; want %q", result.Stdout, "hello")
		}
		if result.Truncated {
			t.Errorf("Truncated = true; want false")
		}
	})
}

func TestResult_TruncatedOnRingPath(t *testing.T) {
	t.Run("false when under ring cap", func(t *testing.T) {
		t.Parallel()
		e := New(WithInheritedEnv())
		result, err := e.Run(context.Background(), "sh", "-c", "printf 'hello\n'")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Truncated {
			t.Errorf("Truncated = true for single-line output; want false")
		}
	})

	t.Run("false at exactly the ring cap", func(t *testing.T) {
		t.Parallel()
		e := New(WithInheritedEnv())
		result, err := e.Run(context.Background(), "sh", "-c",
			"for i in $(seq 1 200); do echo x; done")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Truncated {
			t.Errorf("Truncated = true for exactly 200 lines (nothing dropped); want false")
		}
	})

	t.Run("true when over ring cap", func(t *testing.T) {
		t.Parallel()
		e := New(WithInheritedEnv())
		result, err := e.Run(context.Background(), "sh", "-c",
			"for i in $(seq 1 201); do echo x; done")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Truncated {
			t.Errorf("Truncated = false for 201 lines; want true")
		}
	})
}

func TestStartStreamed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs POSIX sh")
	}

	t.Run("streams output and reports success via done channel", func(t *testing.T) {
		t.Parallel()
		var out strings.Builder
		e := New(WithInheritedEnv(), WithStdout(&out), WithStderr(&strings.Builder{}))

		done, kill, err := e.StartStreamed(context.Background(), "sh", "-c", "printf 'hello\n'")
		if err != nil {
			t.Fatalf("StartStreamed: %v", err)
		}
		defer kill()

		if waitErr := <-done; waitErr != nil {
			t.Fatalf("done channel error: %v", waitErr)
		}
		if !strings.Contains(out.String(), "hello") {
			t.Errorf("streamed stdout missing output; got %q", out.String())
		}
	})

	t.Run("start failure returns error and nil done channel", func(t *testing.T) {
		t.Parallel()
		e := New(WithInheritedEnv())
		done, _, err := e.StartStreamed(context.Background(), "okdctl-definitely-not-a-real-binary")
		if err == nil {
			t.Fatal("expected error for missing binary")
		}
		if done != nil {
			t.Errorf("done channel = %v; want nil on start failure", done)
		}
	})
}

func TestRunOutput_DrainsBeyondPipeBuffer(t *testing.T) {
	t.Parallel()
	e := New(WithInheritedEnv())
	// 2 MiB of output against a 10-byte cap: the excess far exceeds the
	// kernel pipe buffer, so the call only returns if the pipe is drained.
	result, err := e.RunOutput(context.Background(), 10, "sh", "-c",
		"dd if=/dev/zero bs=1024 count=2048 2>/dev/null")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Truncated {
		t.Errorf("Truncated = false; want true")
	}
	if len(result.Stdout) != 10 {
		t.Errorf("len(Stdout) = %d; want 10", len(result.Stdout))
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d; want 0", result.ExitCode)
	}
}
