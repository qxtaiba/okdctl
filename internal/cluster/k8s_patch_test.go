package cluster

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/testutil"
)

// installFakePatchOC installs a POSIX sh "oc" script that logs its full argv
// to OC_ARGV_LOG and exits with OC_EXIT_CODE (default 0).
func installFakePatchOC(t *testing.T) string {
	t.Helper()
	testutil.InstallFakeBin(t, "oc", `#!/bin/sh
echo "$@" >> "$OC_ARGV_LOG"
exit "${OC_EXIT_CODE:-0}"
`)
	argvLog := filepath.Join(t.TempDir(), "argv.log")
	t.Setenv("OC_ARGV_LOG", argvLog)
	return argvLog
}

func TestClientPatch_ArgvShape(t *testing.T) {
	argvLog := installFakePatchOC(t)
	c := New(WithCLI("oc"), WithExecutor(executor.New()))

	err := c.Patch(context.Background(), "operatorhub.config.openshift.io", "cluster", "merge", `{"spec":{"sources":[]}}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, readErr := os.ReadFile(argvLog)
	if readErr != nil {
		t.Fatalf("argv log not written: %v", readErr)
	}
	argv := strings.TrimSpace(string(data))
	want := `patch operatorhub.config.openshift.io cluster --type=merge -p {"spec":{"sources":[]}}`
	if argv != want {
		t.Errorf("argv = %q; want %q", argv, want)
	}
}

func TestClientPatch_NonZeroExitWrapsClusterError(t *testing.T) {
	installFakePatchOC(t)
	t.Setenv("OC_EXIT_CODE", "1")
	c := New(WithCLI("oc"), WithExecutor(executor.New()))

	err := c.Patch(context.Background(), "operatorhub.config.openshift.io", "cluster", "merge", `{}`)
	if err == nil {
		t.Fatal("expected error on non-zero exit")
	}
	var ce *errtypes.ClusterError
	if !errors.As(err, &ce) {
		t.Fatalf("err is %T; want *errtypes.ClusterError", err)
	}
	if !strings.Contains(ce.Msg, "operatorhub.config.openshift.io/cluster") {
		t.Errorf("ClusterError.Msg = %q; want resource/name named", ce.Msg)
	}
}
