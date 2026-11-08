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
		got := defaultEnvAllowlist.allows(tc.key)
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

func TestRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs POSIX sh")
	}

	t.Run("non-zero exit lands in Result", func(t *testing.T) {
		t.Parallel()
		result, err := New().Run(context.Background(), "sh", "-c", "echo broken >&2; exit 7")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.ExitCode != 7 {
			t.Errorf("ExitCode = %d; want 7", result.ExitCode)
		}
		if !strings.Contains(result.Stderr, "broken") {
			t.Errorf("Stderr = %q; want it to contain 'broken'", result.Stderr)
		}
	})

	t.Run("cancelled ctx returns the context error", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		result, err := New().Run(ctx, "sh", "-c", "sleep 10")
		if result == nil {
			t.Fatal("result must be non-nil on error")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v; want context.Canceled", err)
		}
	})
}

func TestNewExitError(t *testing.T) {
	t.Run("live ctx yields ExitError", func(t *testing.T) {
		err := NewExitError(context.Background(), "sh", 3, "oops")
		var exitErr *ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("err type = %T; want *ExitError", err)
		}
		if exitErr.ExitCode != 3 || exitErr.Command != "sh" || exitErr.Stderr != "oops" {
			t.Errorf("ExitError = %+v", *exitErr)
		}
	})

	t.Run("cancelled ctx wins over the exit code", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := NewExitError(ctx, "sh", 3, "oops")
		var exitErr *ExitError
		if errors.As(err, &exitErr) {
			t.Errorf("got *ExitError on ctx cancel; want context error, got: %v", err)
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v; want context.Canceled", err)
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

	result, err := New().RunWithStdin(context.Background(), input, "cat")
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

func TestExitError_Redacted(t *testing.T) {
	e := &ExitError{
		Command:  "openshift-install agent create image",
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
		if !strings.Contains(rendered, "openshift-install agent create image") || !strings.Contains(rendered, "1") {
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
		if !strings.Contains(out, "openshift-install agent create image") {
			t.Errorf("redacted log record lost the command name: %s", out)
		}
	})
}

func TestExitError_Error(t *testing.T) {
	t.Run("truncates long stderr", func(t *testing.T) {
		long := strings.Repeat("x", 500)
		e := &ExitError{Command: "openshift-install", ExitCode: 1, Stderr: long}
		got := e.Error()
		if !strings.Contains(got, "[truncated]") {
			t.Errorf("Error() missing truncation marker for 500-byte stderr; got: %q", got)
		}
		if strings.Contains(got, long) {
			t.Errorf("Error() embedded full 500-byte stderr verbatim")
		}
	})

	t.Run("short stderr passes through", func(t *testing.T) {
		e := &ExitError{Command: "openshift-install", ExitCode: 2, Stderr: "permission denied"}
		got := e.Error()
		if !strings.Contains(got, "permission denied") {
			t.Errorf("Error() dropped short stderr; got: %q", got)
		}
		if strings.Contains(got, "[truncated]") {
			t.Errorf("Error() added truncation marker for short stderr; got: %q", got)
		}
	})
}

func TestResult_TruncatedOnRingPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs POSIX sh")
	}

	t.Run("false at exactly the ring cap", func(t *testing.T) {
		t.Parallel()
		result, err := New().Run(context.Background(), "sh", "-c",
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
		result, err := New().Run(context.Background(), "sh", "-c",
			"for i in $(seq 1 201); do echo x; done")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Truncated {
			t.Errorf("Truncated = false for 201 lines; want true")
		}
	})
}
