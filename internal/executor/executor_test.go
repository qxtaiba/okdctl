package executor

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"
)

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
		{"GITHUB_TOKEN", true},
		{"GH_TOKEN", true},
		{"AWS_SECRET_ACCESS_KEY", false},
		{"DOCKER_PASSWORD", false},
		{"GPG_PRIVATE_KEY", false},
		{"", false},
	}
	for _, tc := range cases {
		got := defaultEnvAllowlist.allows(tc.key)
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
