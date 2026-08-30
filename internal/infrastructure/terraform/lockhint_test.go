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

// lockedExecutor returns an Executor whose workdir has a stale lock file.
func lockedExecutor(t *testing.T) *Executor {
	t.Helper()
	dir := t.TempDir()
	writeLockFile(t, dir)
	return New(dir)
}

func TestWithLockHint_NilErr(t *testing.T) {
	tf := lockedExecutor(t)
	if got := tf.WithLockHint(nil); got != nil {
		t.Errorf("WithLockHint(nil) = %v; want nil", got)
	}
}

func TestWithLockHint_NoLockFile(t *testing.T) {
	tf := New(t.TempDir())
	want := &errtypes.ClusterError{Msg: "boom"}
	if got := tf.WithLockHint(want); !errors.Is(got, want) {
		t.Errorf("WithLockHint() = %v; want the same error instance unchanged", got)
	}
}

// Guards a fixed exit-code flip: joining a ConfigError lock hint into a
// ClusterError used to let errors.As match ConfigError first.
func TestWithLockHint_PreservesConcreteType(t *testing.T) {
	tf := lockedExecutor(t)

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

// Guards a fixed HintAppender gap where Network/Auth/UsageError fell through to
// errors.Join and reclassified to ConfigError.
func TestWithLockHint_NonConfigTypesKeepExitCode(t *testing.T) {
	tf := lockedExecutor(t)

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

func TestWithLockHint_NonErrtypesFallback(t *testing.T) {
	tf := lockedExecutor(t)

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
