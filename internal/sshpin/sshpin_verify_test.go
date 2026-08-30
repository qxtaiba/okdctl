package sshpin

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/testutil"
)

func TestVerify_HappyPath(t *testing.T) {
	testutil.InstallFakeBin(t, "ssh-keyscan", "#!/bin/sh\nprintf '%s\\n' '"+fixtureKeyscanLine+"'\n")
	fp := fingerprintFromFixture(t)

	path, err := Verify(context.Background(), "pve.example", fp, false, logutil.NopLogger)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	assertKnownHosts(t, path)
}

func TestVerify_KeyscanNonZeroExit(t *testing.T) {
	testutil.InstallFakeBin(t, "ssh-keyscan", "#!/bin/sh\nexit 1\n")

	_, err := Verify(context.Background(), "pve.example", "SHA256:doesnotmatter", false, logutil.NopLogger)
	if err == nil {
		t.Fatal("expected error on non-zero ssh-keyscan exit; got nil")
	}
	if !strings.HasPrefix(err.Error(), "ssh-keyscan pve.example:") {
		t.Errorf("error %q missing expected prefix \"ssh-keyscan pve.example:\"", err.Error())
	}
}

func TestVerify_CtxDeadline(t *testing.T) {
	testutil.InstallFakeBin(t, "ssh-keyscan", "#!/bin/sh\nexec sleep 300\n")

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := Verify(ctx, "pve.example", "SHA256:doesnotmatter", false, logutil.NopLogger)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from context deadline; got nil")
	}
	if !strings.HasPrefix(err.Error(), "ssh-keyscan pve.example:") {
		t.Errorf("error %q missing expected prefix \"ssh-keyscan pve.example:\"", err.Error())
	}
	if elapsed > 30*time.Second {
		t.Errorf("Verify took %v under 250ms ctx; expected ctx cancellation to terminate quickly", elapsed)
	}
}
