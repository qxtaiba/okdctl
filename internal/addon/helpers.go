package addon

import (
	"context"
	"errors"
	"fmt"
	"os/exec"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/system"
)

// RetryDefault retries fn under system.DefaultBackoff(). Permanent failures
// (typed config/auth errors, missing binary, ctx cancellation) abort
// immediately; transient failures consume the full backoff budget.
func RetryDefault(ctx context.Context, fn func() error) error {
	return system.Retry(ctx, system.DefaultBackoff(), addonIsRetryable, func(context.Context) error {
		return fn()
	})
}

// addonIsRetryable reports whether err should trigger another retry attempt.
// Permanent failures — typed config/auth errors, missing binary, context
// cancellation — return false so the caller aborts immediately. Transient
// executor failures (non-zero exit, connection errors) return true.
func addonIsRetryable(err error) bool {
	if err == nil {
		return false
	}
	// A system.WaitFor poll timeout wraps DeadlineExceeded but is transient,
	// not caller cancellation; classify it retryable before the ctx arm below
	// so a poll timeout inside a retry closure does not abort the budget.
	if errors.Is(err, errtypes.ErrWaitTimeout) {
		return true
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if errors.Is(err, exec.ErrNotFound) {
		return false
	}
	var cfgErr *errtypes.ConfigError
	if errors.As(err, &cfgErr) {
		return false
	}
	var authErr *errtypes.AuthError
	return !errors.As(err, &authErr)
}

// BuildOpaqueSecret returns a Kubernetes Secret manifest YAML of type Opaque.
// Values in data are raw bytes; they are base64-encoded on marshal by the
// k8s Secret type.
func BuildOpaqueSecret(namespace, name string, data map[string][]byte) (string, error) {
	s := corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}
	out, err := yaml.Marshal(s)
	if err != nil {
		return "", fmt.Errorf("marshal opaque secret %s/%s: %w", namespace, name, err)
	}
	return string(out), nil
}

// EnsureNamespace checks whether a Kubernetes namespace exists and creates it
// if missing, using the default addon retry policy.
func EnsureNamespace(ctx context.Context, env *Environment, namespace string) error {
	// Announce the create once: a transient create failure replays the closure,
	// so log at Info on the first attempt and demote identical retries to Debug
	// per the monitor.go poll-loop log-once convention.
	logged := false
	return RetryDefault(ctx, func() error {
		result, err := env.Exec.Run(ctx, "oc", "get", "namespace", namespace)
		if err != nil {
			// Exec-level failure (command not found, connection refused) —
			// don't attempt create, let retry handle it.
			return fmt.Errorf("cannot reach cluster to check namespace %s: %w", namespace, err)
		}
		if result.ExitCode == 0 {
			return nil
		}

		if logged {
			env.Logger.Debug("creating namespace", "namespace", namespace)
		} else {
			env.Logger.Info("creating namespace", "namespace", namespace)
			logged = true
		}
		if _, err := env.Exec.RunChecked(ctx, "oc", "create", "namespace", namespace); err != nil {
			return fmt.Errorf("create %s namespace: %w", namespace, err)
		}
		return nil
	})
}
