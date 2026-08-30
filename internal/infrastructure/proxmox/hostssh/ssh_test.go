package hostssh

import (
	"context"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/testutil"
)

// installFakeSSHEcho stubs ssh via PATH; it echoes argv space-separated to stdout.
func installFakeSSHEcho(t *testing.T) {
	t.Helper()
	testutil.InstallFakeBin(t, "ssh", "#!/bin/sh\necho \"$@\"\n")
}

func TestSSHRunBaseArgs(t *testing.T) {
	cases := []struct {
		name       string
		knownHosts string
		run        func(exec *executor.Executor, knownHosts string) (*executor.Result, error)
		wants      []string
	}{
		{
			name: "sshRun accept-new",
			run: func(exec *executor.Executor, knownHosts string) (*executor.Result, error) {
				return sshRun(context.Background(), exec, "10.0.0.1", knownHosts, "uptime")
			},
			wants: []string{
				"-o StrictHostKeyChecking=accept-new",
				"-o BatchMode=yes",
				"root@10.0.0.1",
				"uptime",
			},
		},
		{
			name:       "sshRun strict mode",
			knownHosts: "/tmp/known_hosts",
			run: func(exec *executor.Executor, knownHosts string) (*executor.Result, error) {
				return sshRun(context.Background(), exec, "10.0.0.1", knownHosts, "uptime")
			},
			wants: []string{
				"-o UserKnownHostsFile=/tmp/known_hosts",
				"-o StrictHostKeyChecking=yes",
				"-o BatchMode=yes",
				"root@10.0.0.1",
				"uptime",
			},
		},
		{
			name: "SSHRunArgv accept-new",
			run: func(exec *executor.Executor, knownHosts string) (*executor.Result, error) {
				return SSHRunArgv(context.Background(), exec, "10.0.0.2", knownHosts, "pvesh", "get", "/nodes")
			},
			wants: []string{
				"-o StrictHostKeyChecking=accept-new",
				"-o BatchMode=yes",
				"root@10.0.0.2",
				"pvesh",
				"get",
				"/nodes",
			},
		},
		{
			name:       "SSHRunArgv strict mode",
			knownHosts: "/tmp/known_hosts",
			run: func(exec *executor.Executor, knownHosts string) (*executor.Result, error) {
				return SSHRunArgv(context.Background(), exec, "10.0.0.2", knownHosts, "pvesh", "get", "/nodes")
			},
			wants: []string{
				"-o UserKnownHostsFile=/tmp/known_hosts",
				"-o StrictHostKeyChecking=yes",
				"-o BatchMode=yes",
				"root@10.0.0.2",
				"pvesh",
				"get",
				"/nodes",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			installFakeSSHEcho(t)
			result, err := tc.run(executor.New(), tc.knownHosts)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			argv := strings.TrimSpace(result.Stdout)
			for _, want := range tc.wants {
				if !strings.Contains(argv, want) {
					t.Errorf("argv = %q; missing %q", argv, want)
				}
			}
		})
	}
}

// Error must name index/rune but never echo the atom, which could carry credential material.
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

// Pins the full allowed charset so tightening it can't silently break pvesh
// paths, UPIDs, or option atoms.
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
		"[2001:db8::1]:8006":      "2001:db8::1",
		"10.0.0.1":                "10.0.0.1",
		"":                        "",
	}
	for in, want := range cases {
		if got := ProxmoxBareHost(in); got != want {
			t.Errorf("ProxmoxBareHost(%q) = %q, want %q", in, got, want)
		}
	}
}
