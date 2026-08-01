package hostssh

import (
	"context"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/testutil"
)

// goosWindows names the one GOOS every POSIX-shell fake-ssh/find test in
// this package skips on; a shared constant keeps goconst from flagging the
// repeated literal across ssh_test.go, remove_fcos_iso_test.go, and
// iso_cleanup_test.go.
const goosWindows = "windows"

// installFakeSSHEcho writes a POSIX shell script named "ssh" in a temp dir and
// prepends that dir to PATH so the Executor picks it up. The script prints
// all argv to stdout, one space-separated line.
func installFakeSSHEcho(t *testing.T) {
	t.Helper()
	testutil.InstallFakeBin(t, "ssh", "#!/bin/sh\necho \"$@\"\n")
}

func TestSSHRun_acceptNew(t *testing.T) {
	installFakeSSHEcho(t)
	exec := executor.New()

	result, err := sshRun(context.Background(), exec, "10.0.0.1", "", "uptime")
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

func TestSSHRun_strictMode(t *testing.T) {
	installFakeSSHEcho(t)
	exec := executor.New()

	result, err := sshRun(context.Background(), exec, "10.0.0.1", "/tmp/known_hosts", "uptime")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	argv := strings.TrimSpace(result.Stdout)
	for _, want := range []string{
		"-o UserKnownHostsFile=/tmp/known_hosts",
		"-o StrictHostKeyChecking=yes",
		"-o BatchMode=yes",
		"root@10.0.0.1",
		"uptime",
	} {
		if !strings.Contains(argv, want) {
			t.Errorf("argv = %q; missing %q", argv, want)
		}
	}
}

func TestSSHRunArgv_acceptNew(t *testing.T) {
	installFakeSSHEcho(t)
	exec := executor.New()

	result, err := SSHRunArgv(context.Background(), exec, "10.0.0.2", "", "pvesh", "get", "/nodes")
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

func TestSSHRunArgv_strictMode(t *testing.T) {
	installFakeSSHEcho(t)
	exec := executor.New()

	result, err := SSHRunArgv(context.Background(), exec, "10.0.0.2", "/tmp/known_hosts", "pvesh", "get", "/nodes")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	argv := strings.TrimSpace(result.Stdout)
	for _, want := range []string{
		"-o UserKnownHostsFile=/tmp/known_hosts",
		"-o StrictHostKeyChecking=yes",
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

// TestSSHRunArgv_RejectsUnsafeAtoms covers the fail-closed backstop: an
// unvalidated future caller must get an error before ssh runs, not a
// space-joined command string the remote login shell can reinterpret. The
// error must name the index and rune but never echo the atom, which could
// carry credential material.
func TestSSHRunArgv_RejectsUnsafeAtoms(t *testing.T) {
	installFakeSSHEcho(t)
	exec := executor.New()

	payloads := []string{
		"a;reboot",
		"a`id`",
		"a$(reboot)",
		"a|pipe",
		"a&bg",
		"a b",
		"a\tb",
		"a\nb",
		"a'b",
		`a"b`,
		"a\\b",
		"a>o",
		"a<i",
		"a*",
		"a?",
		"a#c",
		"a~c",
		"",
	}
	for _, atom := range payloads {
		for name, run := range map[string]func() (*executor.Result, error){
			"SSHRunArgv": func() (*executor.Result, error) {
				return SSHRunArgv(context.Background(), exec, "10.0.0.2", "", "pvesh", "get", atom)
			},
			"SSHRunArgvOutput": func() (*executor.Result, error) {
				return SSHRunArgvOutput(context.Background(), exec, "10.0.0.2", "", "pvesh", "get", atom)
			},
		} {
			_, err := run()
			if err == nil {
				t.Errorf("%s accepted unsafe atom %q; want error", name, atom)
				continue
			}
			if atom != "" && len(atom) > 2 && strings.Contains(err.Error(), atom) {
				t.Errorf("%s error %q echoes the unsafe atom %q", name, err.Error(), atom)
			}
		}
	}
}

// TestSSHRunArgv_AcceptsShlexSafeCharset pins the full allowed charset so a
// future tightening cannot silently break pvesh paths, UPIDs (: @ .), or
// option atoms.
func TestSSHRunArgv_AcceptsShlexSafeCharset(t *testing.T) {
	installFakeSSHEcho(t)
	exec := executor.New()

	result, err := SSHRunArgv(context.Background(), exec, "10.0.0.2", "",
		"pvesh", "get", "/nodes/pve-01/tasks/UPID:pve:0A:root@pam:/status",
		"--output-format", "a%b+c=d,e._f-g")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(result.Stdout, "a%b+c=d,e._f-g") {
		t.Errorf("argv = %q; safe atom did not pass through", result.Stdout)
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
