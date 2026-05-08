package addon

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/yaml"
)

// Default retry policy shared by all addons using RetryDefault.
const (
	DefaultRetryCount   = 3
	DefaultRetryBackoff = 5 * time.Second
)

// RetryDefault retries fn up to DefaultRetryCount times with exponential
// backoff starting at DefaultRetryBackoff. Context cancellation is checked
// between retries.
func RetryDefault(ctx context.Context, fn func() error) error {
	return wait.ExponentialBackoffWithContext(ctx, wait.Backoff{
		Duration: DefaultRetryBackoff,
		Factor:   2,
		Jitter:   0.5,
		Steps:    DefaultRetryCount,
		Cap:      5 * time.Minute,
	}, func(_ context.Context) (bool, error) {
		// Returning (false, nil) on error asks wait to retry; returning
		// the error would abort the retry loop. Non-retryable errors
		// aren't distinguished today — all fn failures are retried
		// through the full backoff budget, matching the previous
		// cenkalti/backoff behavior.
		if err := fn(); err != nil {
			return false, nil //nolint:nilerr // intentional: retry on any error
		}
		return true, nil
	})
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

		env.Logger.Info("creating namespace", "namespace", namespace)
		if _, err := env.Exec.RunChecked(ctx, "oc", "create", "namespace", namespace); err != nil {
			return fmt.Errorf("failed to create %s namespace: %w", namespace, err)
		}
		return nil
	})
}
