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
	// The hint rides in a structured field, not Msg, and surfaces via Error().
	if clusterErr.Msg != "terraform init failed" {
		t.Errorf("Msg mutated = %q; want the original message unchanged", clusterErr.Msg)
	}
	if !strings.Contains(got.Error(), "force-unlock") {
		t.Errorf("Error() = %q; want lock hint text surfaced", got.Error())
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
	if !strings.Contains(got.Error(), "force-unlock") {
		t.Errorf("Error() = %q; want lock hint text surfaced", got.Error())
	}
}

// TestWithLockHint_NonConfigTypesKeepExitCode locks the fix for the
// HintAppender coverage gap: a NetworkError/AuthError/UsageError used to fall
// through to errors.Join and silently reclassify to the hint's ConfigError
// (exit 2). Every category now implements WithHint, so the concrete type — and
// therefore exitCodeFor's classification — survives, while the hint still
// lands via Error().
func TestWithLockHint_NonConfigTypesKeepExitCode(t *testing.T) {
	dir := t.TempDir()
	writeLockFile(t, dir)
	tf := New(dir)

	cases := []struct {
		name string
		in   error
		want errtypes.Kind
	}{
		{"network", &errtypes.NetworkError{Msg: "registry unreachable"}, errtypes.KindNetwork},
		{"auth", &errtypes.AuthError{Msg: "token rejected"}, errtypes.KindAuth},
		{"usage", &errtypes.UsageError{Msg: "bad flag"}, errtypes.KindUsage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tf.WithLockHint(tc.in)
			kind, ok := errtypes.Classify(got)
			if !ok || kind != tc.want {
				t.Fatalf("Classify = %v (ok=%v); want %v — type reclassified by lock hint",
					kind, ok, tc.want)
			}
			var cfgErr *errtypes.ConfigError
			if errors.As(got, &cfgErr) {
				t.Errorf("err also matches *errtypes.ConfigError (%v); exit code would flip to 2", cfgErr)
			}
			if !strings.Contains(got.Error(), "force-unlock") {
				t.Errorf("Error() = %q; want lock hint text surfaced", got.Error())
			}
		})
	}
}

// TestWithLockHint_NonErrtypesFallback locks the plain-text fallback: a raw
// (non-errtypes) error keeps its identity through %w so errors.Is still
// matches it, and no *errtypes.ConfigError is introduced into the chain.
func TestWithLockHint_NonErrtypesFallback(t *testing.T) {
	dir := t.TempDir()
	writeLockFile(t, dir)
	tf := New(dir)

	raw := errors.New("terraform apply exited 1")
	got := tf.WithLockHint(raw)

	if !errors.Is(got, raw) {
		t.Errorf("errors.Is(got, raw) = false; %%w must preserve the raw error identity")
	}
	var cfgErr *errtypes.ConfigError
	if errors.As(got, &cfgErr) {
		t.Errorf("fallback introduced *errtypes.ConfigError (%v); exit code would flip to 2", cfgErr)
	}
	if !strings.Contains(got.Error(), "force-unlock") {
		t.Errorf("Error() = %q; want lock hint text surfaced", got.Error())
	}
}
