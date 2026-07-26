package provision

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/testutil"
)

func installFakeSCP(t *testing.T) {
	t.Helper()
	script := `#!/bin/sh
for arg in "$@"; do
  printf '%s\n' "$arg"
done
exit 0
`
	testutil.InstallFakeBin(t, "scp", script)
}

func newUploadExecutor(extra ...executor.Option) *executor.Executor {
	opts := append([]executor.Option{
		executor.WithInheritedEnv(),
		executor.WithLogger(logutil.NopLogger),
	}, extra...)
	return executor.New(opts...)
}

func TestUploadISOsViaSCP_argvShape(t *testing.T) {
	installFakeSCP(t)

	const (
		host       = "pve.example"
		remotePath = "/srv/custom/iso"
	)
	isoFiles := []string{
		"/tmp/isos/coreos.iso",
		"/tmp/isos/custom.iso",
	}

	var buf bytes.Buffer
	exec := newUploadExecutor(executor.WithStdout(&buf))

	if err := uploadISOsViaSCP(context.Background(), exec, isoFiles, host, remotePath, ""); err != nil {
		t.Fatalf("uploadISOsViaSCP: %v", err)
	}

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")

	found := false
	for i, l := range lines {
		if l == "-o" && i+1 < len(lines) && lines[i+1] == "StrictHostKeyChecking=accept-new" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("argv missing -o StrictHostKeyChecking=accept-new pair; got: %v", lines)
	}

	for _, iso := range isoFiles {
		seen := false
		for _, l := range lines {
			if l == iso {
				seen = true
				break
			}
		}
		if !seen {
			t.Errorf("iso path %q missing as discrete argv entry; got: %v", iso, lines)
		}
	}

	want := proxmoxSCPUser + "@" + host + ":" + remotePath + "/"
	dest := false
	for _, l := range lines {
		if l == want {
			dest = true
			break
		}
	}
	if !dest {
		t.Errorf("destination %q missing or malformed; got: %v", want, lines)
	}
}

func TestUploadISOsViaSCP_pinnedUsesStrictChecking(t *testing.T) {
	installFakeSCP(t)
	const (
		host           = "pve.example"
		remotePath     = "/var/lib/vz/template/iso"
		knownHostsPath = "/tmp/okdctl-known-hosts-pinned"
	)
	isoFiles := []string{"/tmp/isos/coreos.iso"}

	var buf bytes.Buffer
	exec := newUploadExecutor(executor.WithStdout(&buf))

	if err := uploadISOsViaSCP(context.Background(), exec, isoFiles, host, remotePath, knownHostsPath); err != nil {
		t.Fatalf("uploadISOsViaSCP: %v", err)
	}

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")

	wantOpts := []string{
		"UserKnownHostsFile=" + knownHostsPath,
		"StrictHostKeyChecking=yes",
		"BatchMode=yes",
	}
	for _, opt := range wantOpts {
		found := false
		for i, l := range lines {
			if l == "-o" && i+1 < len(lines) && lines[i+1] == opt {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("argv missing -o %s pair; got: %v", opt, lines)
		}
	}
}

func TestUploadISOsViaSCP_spaceInFilename(t *testing.T) {
	installFakeSCP(t)

	const (
		host       = "pve.example"
		remotePath = "/var/lib/vz/template/iso"
	)
	spaced := "/tmp/isos/my coreos image.iso"
	isoFiles := []string{spaced}

	var buf bytes.Buffer
	exec := newUploadExecutor(executor.WithStdout(&buf))

	if err := uploadISOsViaSCP(context.Background(), exec, isoFiles, host, remotePath, ""); err != nil {
		t.Fatalf("uploadISOsViaSCP: %v", err)
	}

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")

	found := false
	for _, l := range lines {
		if l == spaced {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("spaced filename %q not found as single argv entry; got lines: %v", spaced, lines)
	}
}
