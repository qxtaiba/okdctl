package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

// wantExitError fails t unless err unwraps to *ExitError with the given
// exit code, returning it for further field assertions.
func wantExitError(t *testing.T, err error, code int) *ExitError {
	t.Helper()
	if err == nil {
		t.Fatalf("expected *ExitError with exit %d; got nil", code)
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("err type = %T; want *ExitError", err)
	}
	if exitErr.ExitCode != code {
		t.Errorf("ExitCode = %d; want %d", exitErr.ExitCode, code)
	}
	return exitErr
}

// wantCtxErrorNotExitError fails t when err is nil or unwraps to *ExitError —
// a ctx-cancelled run must surface the context error instead.
func wantCtxErrorNotExitError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error after ctx cancel; got nil")
	}
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		t.Errorf("got *ExitError on ctx cancel; want context error, got: %v", err)
	}
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
		exitErr := wantExitError(t, err, 3)
		if !strings.Contains(exitErr.Stderr, "oops") {
			t.Errorf("Stderr = %q; want it to contain 'oops'", exitErr.Stderr)
		}
	})
}

func TestBuildEnv_AllowlistFilters(t *testing.T) {
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("HOME", "/tmp/home")
	t.Setenv("KUBECONFIG", "/tmp/kube")
	t.Setenv("KUBE_PS1", "on")                  // prefix match
	t.Setenv("TF_VAR_region", "us-east-1")      // prefix match
	t.Setenv("PROXMOX_VE_ENDPOINT", "pve:8006") // prefix match, non-secret
	t.Setenv("PROXMOX_VE_PASSWORD", "hunter")   // prefix match, secret-keyed
	t.Setenv("AWS_SECRET_ACCESS_KEY", "nope")   // not in allowlist
	t.Setenv("SECRET_API_KEY", "nope")          // not in allowlist
	t.Setenv("OKDCTL_INTERNAL_TOKEN", "nope")   // not in allowlist

	e := New()
	env := e.buildEnv()
	joined := "\x00" + strings.Join(env, "\x00") + "\x00"

	mustContain := []string{
		"PATH=/usr/bin:/bin",
		"HOME=/tmp/home",
		"KUBECONFIG=/tmp/kube",
		"KUBE_PS1=on",
		"TF_VAR_region=us-east-1",
		"PROXMOX_VE_ENDPOINT=pve:8006",
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
		"PROXMOX_VE_PASSWORD=",
	}
	for _, forbid := range mustNotContain {
		if strings.Contains(joined, "\x00"+forbid) {
			t.Errorf("%q leaked through allowlist; env:\n%v", forbid, env)
		}
	}
}

func TestBuildEnv_SecretKeyedDroppedFromParentButNotWithEnv(t *testing.T) {
	t.Setenv("PROXMOX_VE_PASSWORD", "from-parent")
	t.Setenv("PROXMOX_VE_API_TOKEN", "from-parent")
	t.Setenv("TF_VAR_db_password", "from-parent")

	e := New(WithEnv([]string{"PROXMOX_VE_PASSWORD=from-caller"}))
	joined := "\n" + strings.Join(e.buildEnv(), "\n") + "\n"

	for _, forbid := range []string{
		"\nPROXMOX_VE_PASSWORD=from-parent\n",
		"\nPROXMOX_VE_API_TOKEN=from-parent\n",
		"\nTF_VAR_db_password=from-parent\n",
	} {
		if strings.Contains(joined, forbid) {
			t.Errorf("parent secret %q leaked into subprocess env:\n%s", strings.TrimSpace(forbid), joined)
		}
	}
	if !strings.Contains(joined, "\nPROXMOX_VE_PASSWORD=from-caller\n") {
		t.Errorf("WithEnv-supplied credential missing from env:\n%s", joined)
	}
}

func TestBuildEnv_WithEnvAppendsAfterAllowlist(t *testing.T) {
	t.Setenv("KUBECONFIG", "/parent/kube")
	e := New(WithEnv([]string{"KUBECONFIG=/caller/kube", "CUSTOM_VAR=value"}))

	env := e.buildEnv()
	// Caller override must appear after the parent entry — os/exec uses the
	// last occurrence.
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "KUBECONFIG=/caller/kube") {
		t.Errorf("caller KUBECONFIG missing: %v", env)
	}
	if !strings.Contains(joined, "CUSTOM_VAR=value") {
		t.Errorf("caller CUSTOM_VAR missing: %v", env)
	}
	parentIdx := strings.Index(joined, "KUBECONFIG=/parent/kube")
	callerIdx := strings.Index(joined, "KUBECONFIG=/caller/kube")
	if parentIdx < 0 || callerIdx < 0 || callerIdx < parentIdx {
		t.Errorf("order wrong — parent=%d caller=%d, want caller > parent",
			parentIdx, callerIdx)
	}
}

func TestBuildEnv_InheritedNilWhenNoCallerEnv(t *testing.T) {
	// nil env is the os/exec contract for inheriting os.Environ() verbatim.
	e := New(WithInheritedEnv())
	env := e.buildEnv()
	if env != nil {
		t.Errorf("expected nil env (os/exec contract for full inheritance); got %v", env)
	}
}

func TestBuildEnv_InheritedWithAppendedVars(t *testing.T) {
	t.Setenv("OKDCTL_INTERNAL_TOKEN", "inherited")
	// Combining WithInheritedEnv+WithEnv must materialize os.Environ() —
	// nil would drop the caller vars.
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
		{"KUBECONFIG", true},
		{"KUBE_PS1", true},
		{"TF_VAR_thing", true},
		{"PROXMOX_VE_API_TOKEN", true},
		{"OC_EDITOR", true},
		{"HELM_CACHE_HOME", true},
		{"GIT_SSH_COMMAND", true},
		{"GITHUB_TOKEN", false},
		{"GH_TOKEN", false},
		{"AWS_SECRET_ACCESS_KEY", false},
		{"", false},
	}
	for _, tc := range cases {
		got := DefaultEnvAllowlist.allows(tc.key)
		if got != tc.want {
			t.Errorf("allows(%q) = %v; want %v", tc.key, got, tc.want)
		}
	}
}

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
}

func TestRunWithStdin_RoundTrip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs POSIX cat")
	}
	t.Parallel()

	// 100×~2KiB exceeds the 64KiB pipe buffer — the call only returns if
	// stdin is fully drained.
	line := strings.Repeat("x", 2000)
	var b strings.Builder
	for range 100 {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	input := b.String()

	e := New(WithInheritedEnv())
	result, err := e.RunWithStdin(context.Background(), input, "cat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d; want 0", result.ExitCode)
	}
	if got, want := result.Stdout, strings.TrimSuffix(input, "\n"); got != want {
		t.Errorf("stdout != stdin round-trip: len(got)=%d len(want)=%d", len(got), len(want))
	}
}

func TestRunChecked(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs POSIX sh")
	}

	t.Run("zero exit returns nil error", func(t *testing.T) {
		t.Parallel()
		e := New(WithInheritedEnv())
		result, err := e.RunChecked(context.Background(), "sh", "-c", "printf ok")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Stdout != "ok" {
			t.Errorf("Stdout = %q; want %q", result.Stdout, "ok")
		}
	})

	t.Run("non-zero exit returns typed ExitError with stderr", func(t *testing.T) {
		t.Parallel()
		e := New(WithInheritedEnv())
		result, err := e.RunChecked(context.Background(), "sh", "-c", "echo broken >&2; exit 7")
		exitErr := wantExitError(t, err, 7)
		if exitErr.Command != "sh" {
			t.Errorf("Command = %q; want %q", exitErr.Command, "sh")
		}
		if !strings.Contains(exitErr.Stderr, "broken") {
			t.Errorf("Stderr = %q; want it to contain 'broken'", exitErr.Stderr)
		}
		if result == nil {
			t.Fatal("result must be non-nil on error")
		}
	})

	t.Run("ctx cancel returns context error not ExitError", func(t *testing.T) {
		t.Parallel()
		e := New(WithInheritedEnv())
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := e.RunChecked(ctx, "sh", "-c", "sleep 10")
		wantCtxErrorNotExitError(t, err)
	})
}

func TestRunWithStdinChecked(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs POSIX sh")
	}

	t.Run("non-zero exit returns typed ExitError", func(t *testing.T) {
		t.Parallel()
		e := New(WithInheritedEnv())
		_, err := e.RunWithStdinChecked(context.Background(), "ignored", "sh", "-c",
			"cat >/dev/null; echo apply-failed >&2; exit 1")
		_ = wantExitError(t, err, 1)
	})
}

func TestAppendEnv(t *testing.T) {
	t.Run("appended entry reaches the child", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("needs POSIX env")
		}
		e := New()
		e.AppendEnv("KUBECONFIG=/appended/kube", "CUSTOM_APPENDED_VAR=yes")
		result, err := e.Run(context.Background(), "env")
		if err != nil {
			t.Fatalf("env: %v", err)
		}
		// Allowlist filters the parent env only — explicitly appended keys
		// must still pass through.
		for _, want := range []string{"KUBECONFIG=/appended/kube", "CUSTOM_APPENDED_VAR=yes"} {
			if !strings.Contains(result.Stdout, want) {
				t.Errorf("appended entry %q missing from child env:\n%s", want, result.Stdout)
			}
		}
	})

	t.Run("ZeroizeEnv blanks secret-keyed appended entry", func(t *testing.T) {
		e := New()
		e.AppendEnv("PROXMOX_VE_PASSWORD=hunter2", "KUBECONFIG=/etc/kube")
		snap := e.env
		e.ZeroizeEnv()
		if e.env != nil {
			t.Errorf("env not nil after ZeroizeEnv; got %v", e.env)
		}
		for i, entry := range snap {
			if entry != "" {
				t.Errorf("backing entry %d not blanked after ZeroizeEnv; got %q", i, entry)
			}
		}
	})
}

func TestExitError_Redacted(t *testing.T) {
	e := &ExitError{
		Command:  "terraform apply",
		ExitCode: 1,
		Stderr:   "provider auth: password=hunter2",
	}

	t.Run("omits Stderr from the redacted shape", func(t *testing.T) {
		red := e.Redacted()
		data, err := json.Marshal(red)
		if err != nil {
			t.Fatalf("marshal redacted value: %v", err)
		}
		if strings.Contains(string(data), "hunter2") {
			t.Errorf("Redacted() leaks stderr credential: %s", data)
		}
		if strings.Contains(string(data), "Stderr") {
			t.Errorf("Redacted() must omit the Stderr field entirely: %s", data)
		}
		rendered := fmt.Sprintf("%+v", red)
		if !strings.Contains(rendered, "terraform apply") || !strings.Contains(rendered, "1") {
			t.Errorf("Redacted() dropped command identity: %q", rendered)
		}
	})

	t.Run("slog sink through RedactHandler never sees stderr", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(logutil.NewRedactHandler(slog.NewJSONHandler(&buf, nil)))
		logger.Warn("subprocess failed", "err", e)
		out := buf.String()
		if strings.Contains(out, "hunter2") {
			t.Errorf("credential-bearing stderr reached the slog sink: %s", out)
		}
		if !strings.Contains(out, "terraform apply") {
			t.Errorf("redacted log record lost the command name: %s", out)
		}
	})
}

func TestExitError_Error(t *testing.T) {
	t.Run("truncates long stderr", func(t *testing.T) {
		long := strings.Repeat("x", 500)
		e := &ExitError{Command: "terraform apply", ExitCode: 1, Stderr: long}
		got := e.Error()
		if !strings.Contains(got, "[truncated]") {
			t.Errorf("Error() missing truncation marker for 500-byte stderr; got: %q", got)
		}
		if strings.Contains(got, long) {
			t.Errorf("Error() embedded full 500-byte stderr verbatim")
		}
	})

	t.Run("short stderr passes through", func(t *testing.T) {
		e := &ExitError{Command: "terraform plan", ExitCode: 2, Stderr: "permission denied"}
		got := e.Error()
		if !strings.Contains(got, "permission denied") {
			t.Errorf("Error() dropped short stderr; got: %q", got)
		}
		if strings.Contains(got, "[truncated]") {
			t.Errorf("Error() added truncation marker for short stderr; got: %q", got)
		}
	})
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
		exitErr := wantExitError(t, err, 1)
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
		wantCtxErrorNotExitError(t, err)
	})
}

func TestRunInteractive_CtxCancelReturnsCtxErr(t *testing.T) {
	t.Parallel()

	e := New(WithInheritedEnv(), WithStdout(&strings.Builder{}), WithStderr(&strings.Builder{}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := e.RunInteractive(ctx, "sh", "-c", "sleep 10")
	wantCtxErrorNotExitError(t, err)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v; want errors.Is(err, context.Canceled) == true", err)
	}
}

func TestRunOutput_FullCapture(t *testing.T) {
	t.Run("captures more than maxCapturedLines lines", func(t *testing.T) {
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
		_ = wantExitError(t, err, 2)
		if result == nil {
			t.Fatal("result must be non-nil on error")
		}
		if !strings.Contains(result.Stdout, "out") {
			t.Errorf("result.Stdout missing captured output; got %q", result.Stdout)
		}
	})
}

func TestResult_TruncatedOnRingPath(t *testing.T) {
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
	// 2MiB output vs a 10-byte cap exceeds the pipe buffer — returns only
	// if fully drained.
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
