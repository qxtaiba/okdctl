package provision

import (
	"bytes"
	"context"
	"slices"
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

func runFakeSCPUpload(t *testing.T, isoFiles []string, host, remotePath, knownHostsPath string) []string {
	t.Helper()
	installFakeSCP(t)

	var buf bytes.Buffer
	exec := newUploadExecutor(executor.WithStdout(&buf))

	if err := uploadISOsViaSCP(context.Background(), exec, isoFiles, host, remotePath, knownHostsPath); err != nil {
		t.Fatalf("uploadISOsViaSCP: %v", err)
	}
	return strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
}

func assertDashOPair(t *testing.T, lines []string, opt string) {
	t.Helper()
	for i, l := range lines {
		if l == "-o" && i+1 < len(lines) && lines[i+1] == opt {
			return
		}
	}
	t.Errorf("argv missing -o %s pair; got: %v", opt, lines)
}

func TestUploadISOsViaSCP_argvShape(t *testing.T) {
	const (
		host       = "pve.example"
		remotePath = "/srv/custom/iso"
	)
	isoFiles := []string{
		"/tmp/isos/coreos.iso",
		"/tmp/isos/custom.iso",
	}

	lines := runFakeSCPUpload(t, isoFiles, host, remotePath, "")

	assertDashOPair(t, lines, "StrictHostKeyChecking=accept-new")

	for _, iso := range isoFiles {
		if !slices.Contains(lines, iso) {
			t.Errorf("iso path %q missing as discrete argv entry; got: %v", iso, lines)
		}
	}

	want := proxmoxSCPUser + "@" + host + ":" + remotePath + "/"
	if !slices.Contains(lines, want) {
		t.Errorf("destination %q missing or malformed; got: %v", want, lines)
	}
}

func TestUploadISOsViaSCP_pinnedUsesStrictChecking(t *testing.T) {
	const (
		host           = "pve.example"
		remotePath     = "/var/lib/vz/template/iso"
		knownHostsPath = "/tmp/okdctl-known-hosts-pinned"
	)

	lines := runFakeSCPUpload(t, []string{"/tmp/isos/coreos.iso"}, host, remotePath, knownHostsPath)

	for _, opt := range []string{
		"UserKnownHostsFile=" + knownHostsPath,
		"StrictHostKeyChecking=yes",
		"BatchMode=yes",
	} {
		assertDashOPair(t, lines, opt)
	}
}

func TestUploadISOsViaSCP_spaceInFilename(t *testing.T) {
	spaced := "/tmp/isos/my coreos image.iso"

	lines := runFakeSCPUpload(t, []string{spaced}, "pve.example", "/var/lib/vz/template/iso", "")

	if !slices.Contains(lines, spaced) {
		t.Errorf("spaced filename %q not found as single argv entry; got lines: %v", spaced, lines)
	}
}
