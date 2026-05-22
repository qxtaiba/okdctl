package sshpin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

// installFakeKeyscan writes a POSIX sh script named "ssh-keyscan" to a temp
// dir and prepends it to PATH. script must begin with "#!/bin/sh\n".
func installFakeKeyscan(t *testing.T, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake ssh-keyscan script requires POSIX sh")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ssh-keyscan"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestVerify_HappyPath(t *testing.T) {
	installFakeKeyscan(t, "#!/bin/sh\nprintf '%s\\n' '"+fixtureKeyscanLine+"'\n")
	fp := fingerprintFromFixture(t)

	path, err := Verify(context.Background(), "pve.example", fp, false, logutil.NopLogger)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if path == "" {
		t.Fatal("want non-empty known_hosts path on match; got empty string")
	}
	defer func() { _ = os.Remove(path) }()
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("cannot read known_hosts at %s: %v", path, readErr)
	}
	if !strings.Contains(string(content), "ssh-ed25519") {
		t.Errorf("known_hosts content %q missing key type", string(content))
	}
}

func TestVerify_KeyscanNonZeroExit(t *testing.T) {
	installFakeKeyscan(t, "#!/bin/sh\nexit 1\n")

	_, err := Verify(context.Background(), "pve.example", "SHA256:doesnotmatter", false, logutil.NopLogger)
	if err == nil {
		t.Fatal("expected error on non-zero ssh-keyscan exit; got nil")
	}
	if !strings.HasPrefix(err.Error(), "ssh-keyscan pve.example:") {
		t.Errorf("error %q missing expected prefix \"ssh-keyscan pve.example:\"", err.Error())
	}
}

func TestVerify_CtxDeadline(t *testing.T) {
	installFakeKeyscan(t, "#!/bin/sh\nexec sleep 300\n")

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	_, err := Verify(ctx, "pve.example", "SHA256:doesnotmatter", false, logutil.NopLogger)
	if err == nil {
		t.Fatal("expected error from context deadline; got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v; want errors.Is(_, context.DeadlineExceeded)", err)
	}
}
