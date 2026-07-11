package terraform

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

func writeLockFile(t *testing.T, dir string) {
	t.Helper()
	lockPath := filepath.Join(dir, ".terraform.tfstate.lock.info")
	if err := os.WriteFile(lockPath, []byte(`{"ID":"abc"}`), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestWithLockHint_NilErr locks that WithLockHint is nil-in/nil-out even
// when a stale lock file is present.
func TestWithLockHint_NilErr(t *testing.T) {
	dir := t.TempDir()
	writeLockFile(t, dir)
	tf := New(dir)
	if got := tf.WithLockHint(nil); got != nil {
		t.Errorf("WithLockHint(nil) = %v; want nil", got)
	}
}

// TestWithLockHint_NoLockFile locks that err passes through unchanged when
// no lock file is present.
func TestWithLockHint_NoLockFile(t *testing.T) {
	tf := New(t.TempDir())
	want := &errtypes.ClusterError{Msg: "boom"}
	if got := tf.WithLockHint(want); !errors.Is(got, want) {
		t.Errorf("WithLockHint() = %v; want the same error instance unchanged", got)
	}
}

// TestWithLockHint_PreservesConcreteType locks the fix for the exit-code
// flip: joining a *errtypes.ConfigError lock hint into a *errtypes.ClusterError
// used to let errors.As match ConfigError first, flipping the process exit
// code for the same underlying terraform failure depending on lock state.
// WithLockHint must keep err's concrete type so exitCodeFor's classification
// stays stable.
func TestWithLockHint_PreservesConcreteType(t *testing.T) {
	dir := t.TempDir()
	writeLockFile(t, dir)
	tf := New(dir)

	got := tf.WithLockHint(&errtypes.ClusterError{Msg: "terraform init failed"})

	var clusterErr *errtypes.ClusterError
	if !errors.As(got, &clusterErr) {
		t.Fatalf("err = %v; want *errtypes.ClusterError", got)
	}
	var cfgErr *errtypes.ConfigError
	if errors.As(got, &cfgErr) {
		t.Errorf("err also matches *errtypes.ConfigError (%v); exitCodeFor would misclassify it", cfgErr)
	}
	if !strings.Contains(clusterErr.Msg, "force-unlock") {
		t.Errorf("Msg = %q; want lock hint text embedded", clusterErr.Msg)
	}
}

// TestWithLockHint_ConfigErrorType covers the destroy --dry-run call sites:
// same-type join has no flip risk, but the hint must still land and the
// type must stay *errtypes.ConfigError.
func TestWithLockHint_ConfigErrorType(t *testing.T) {
	dir := t.TempDir()
	writeLockFile(t, dir)
	tf := New(dir)

	got := tf.WithLockHint(&errtypes.ConfigError{Msg: "terraform init failed in dry-run"})

	var cfgErr *errtypes.ConfigError
	if !errors.As(got, &cfgErr) {
		t.Fatalf("err = %v; want *errtypes.ConfigError", got)
	}
	if !strings.Contains(cfgErr.Msg, "force-unlock") {
		t.Errorf("Msg = %q; want lock hint text embedded", cfgErr.Msg)
	}
}
