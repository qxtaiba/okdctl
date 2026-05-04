package setup

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

func installFakeSCP(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-scp script relies on POSIX sh")
	}
	dir := t.TempDir()
	script := `#!/bin/sh
for arg in "$@"; do
  printf '%s\n' "$arg"
done
exit 0
`
	path := filepath.Join(dir, "scp")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func newUploadExecutor() *executor.Executor {
	return executor.New(
		executor.WithInheritedEnv(),
		executor.WithLogger(logutil.NopLogger),
	)
}

func TestUploadISOsViaSCP_argvShape(t *testing.T) {
	installFakeSCP(t)

	const (
		user       = "root"
		host       = "pve.example"
		remotePath = "/var/lib/vz/template/iso"
	)
	isoFiles := []string{
		"/tmp/isos/coreos.iso",
		"/tmp/isos/custom.iso",
	}

	var buf bytes.Buffer
	exec := newUploadExecutor()
	exec.Stdout = &buf

	if err := uploadISOsViaSCP(context.Background(), exec, isoFiles, user, host, remotePath); err != nil {
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

	want := user + "@" + host + ":" + remotePath + "/"
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

func TestUploadISOsViaSCP_spaceInFilename(t *testing.T) {
	installFakeSCP(t)

	const (
		user       = "root"
		host       = "pve.example"
		remotePath = "/var/lib/vz/template/iso"
	)
	spaced := "/tmp/isos/my coreos image.iso"
	isoFiles := []string{spaced}

	var buf bytes.Buffer
	exec := newUploadExecutor()
	exec.Stdout = &buf

	if err := uploadISOsViaSCP(context.Background(), exec, isoFiles, user, host, remotePath); err != nil {
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
