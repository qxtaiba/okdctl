package cluster

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/testutil"
)

// installFakeOCForCSRs installs a PATH-shadow "oc" keyed off $OC_CSR_JSON,
// $OC_GET_EXIT, $OC_ARGV_FILE, $OC_APPROVE_EXIT.
func installFakeOCForCSRs(t *testing.T) {
	t.Helper()
	script := `#!/bin/sh
default_csr='{"items":[]}'
case "$1" in
  get)
    if [ -z "${OC_CSR_JSON:-}" ]; then
      printf '%s' "$default_csr"
    else
      printf '%s' "$OC_CSR_JSON"
    fi
    exit "${OC_GET_EXIT:-0}"
    ;;
  adm)
    if [ -n "${OC_ARGV_FILE:-}" ]; then
      echo "$@" >> "${OC_ARGV_FILE}"
    fi
    exit "${OC_APPROVE_EXIT:-0}"
    ;;
  *)
    exit 0
    ;;
esac
`
	testutil.InstallFakeBin(t, "oc", script)
}

func newTestClient(t *testing.T) *Client {
	t.Helper()
	return New(
		WithCLI("oc"),
		WithLogger(logutil.NopLogger),
	)
}

func TestApprovePendingCSRs_EmptyList(t *testing.T) {
	installFakeOCForCSRs(t)
	dir := t.TempDir()
	argvFile := filepath.Join(dir, "argv")
	t.Setenv("OC_ARGV_FILE", argvFile)
	t.Setenv("OC_CSR_JSON", `{"items":[]}`)

	c := newTestClient(t)
	n, err := c.ApprovePendingCSRs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("approved count = %d; want 0", n)
	}
	if _, statErr := os.Stat(argvFile); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("approve was called unexpectedly (argv file exists)")
	}
}

func TestApprovePendingCSRs_BatchedSingleCall(t *testing.T) {
	installFakeOCForCSRs(t)
	dir := t.TempDir()
	argvFile := filepath.Join(dir, "argv")
	t.Setenv("OC_ARGV_FILE", argvFile)
	t.Setenv("OC_CSR_JSON", `{"items":[`+
		`{"metadata":{"name":"csr-1"},"status":{"conditions":[]}},`+
		`{"metadata":{"name":"csr-2"},"status":{"conditions":[]}},`+
		`{"metadata":{"name":"csr-3"},"status":{"conditions":[]}}]}`)

	c := newTestClient(t)
	n, err := c.ApprovePendingCSRs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 3 {
		t.Errorf("approved count = %d; want 3", n)
	}
	data, readErr := os.ReadFile(argvFile)
	if readErr != nil {
		t.Fatalf("argv file not written: %v", readErr)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 1 {
		t.Errorf("approve called %d times; want exactly 1 (batched)", len(lines))
	}
	for _, name := range []string{"csr-1", "csr-2", "csr-3"} {
		if !strings.Contains(lines[0], name) {
			t.Errorf("argv line %q missing CSR name %q", lines[0], name)
		}
	}
}

func TestApprovePendingCSRs_PendingCSRsError(t *testing.T) {
	installFakeOCForCSRs(t)
	t.Setenv("OC_GET_EXIT", "1")

	c := newTestClient(t)
	n, err := c.ApprovePendingCSRs(context.Background())
	if err == nil {
		t.Fatal("expected error when get csr exits non-zero")
	}
	if n != 0 {
		t.Errorf("approved count = %d; want 0 on error", n)
	}
	var ce *errtypes.ClusterError
	if !errors.As(err, &ce) {
		t.Fatalf("err is %T; want *errtypes.ClusterError", err)
	}
}

func TestApprovePendingCSRs_ApproveFailureWrapped(t *testing.T) {
	installFakeOCForCSRs(t)
	t.Setenv("OC_CSR_JSON", `{"items":[{"metadata":{"name":"csr-1"},"status":{"conditions":[]}}]}`)
	t.Setenv("OC_APPROVE_EXIT", "1")

	c := newTestClient(t)
	n, err := c.ApprovePendingCSRs(context.Background())
	if err == nil {
		t.Fatal("expected error when approve exits non-zero")
	}
	if n != 0 {
		t.Errorf("approved count = %d; want 0 on failure", n)
	}
	var ce *errtypes.ClusterError
	if !errors.As(err, &ce) {
		t.Fatalf("err is %T; want *errtypes.ClusterError", err)
	}
	if !strings.HasPrefix(ce.Msg, "approve CSRs") {
		t.Errorf("ClusterError.Msg = %q; want prefix %q", ce.Msg, "approve CSRs")
	}
}
