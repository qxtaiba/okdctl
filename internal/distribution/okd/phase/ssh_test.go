package phase

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/executor"
)

// installFakeSSHEcho writes a POSIX shell script named "ssh" in a temp dir and
// prepends that dir to PATH so the Executor picks it up. The script prints
// all argv to stdout, one space-separated line.
func installFakeSSHEcho(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-ssh script relies on POSIX sh")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\necho \"$@\"\n"
	path := filepath.Join(dir, "ssh")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestSSHRun(t *testing.T) {
	installFakeSSHEcho(t)
	exec := executor.New()

	result, err := SSHRun(context.Background(), exec, "10.0.0.1", "uptime")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	argv := strings.TrimSpace(result.Stdout)
	for _, want := range []string{
		"-o StrictHostKeyChecking=accept-new",
		"-o BatchMode=yes",
		"root@10.0.0.1",
		"uptime",
	} {
		if !strings.Contains(argv, want) {
			t.Errorf("argv = %q; missing %q", argv, want)
		}
	}
}

func TestSSHRunArgv(t *testing.T) {
	installFakeSSHEcho(t)
	exec := executor.New()

	result, err := SSHRunArgv(context.Background(), exec, "10.0.0.2", "pvesh", "get", "/nodes")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	argv := strings.TrimSpace(result.Stdout)
	for _, want := range []string{
		"-o StrictHostKeyChecking=accept-new",
		"-o BatchMode=yes",
		"root@10.0.0.2",
		"pvesh",
		"get",
		"/nodes",
	} {
		if !strings.Contains(argv, want) {
			t.Errorf("argv = %q; missing %q", argv, want)
		}
	}
}

func TestProxmoxBareHost(t *testing.T) {
	cases := map[string]string{
		"pve.example":             "pve.example",
		"pve.example:8006":        "pve.example",
		"https://pve.example":     "pve.example",
		"https://pve.example:443": "pve.example",
		"http://pve.example:8006": "pve.example",
		"[2001:db8::1]:8006":      "2001:db8::1",
		"10.0.0.1":                "10.0.0.1",
		"10.0.0.1:22":             "10.0.0.1",
		"":                        "",
	}
	for in, want := range cases {
		if got := ProxmoxBareHost(in); got != want {
			t.Errorf("ProxmoxBareHost(%q) = %q, want %q", in, got, want)
		}
	}
}
