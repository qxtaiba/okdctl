package addon

import (
	"context"
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v4"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

const (
	DefaultRetryCount   = 3
	DefaultRetryBackoff = 5 * time.Second
)

// RetryDefault retries fn up to DefaultRetryCount times with exponential
// backoff starting at DefaultRetryBackoff. Context cancellation is checked
// between retries.
func RetryDefault(ctx context.Context, fn func() error) error {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = DefaultRetryBackoff
	b.Multiplier = 2
	b.RandomizationFactor = 0.5
	b.MaxInterval = 5 * time.Minute
	b.MaxElapsedTime = 0

	return backoff.Retry(fn, backoff.WithContext(
		backoff.WithMaxRetries(b, uint64(DefaultRetryCount-1)),
		ctx,
	))
}

// BuildOpaqueSecret returns a Kubernetes Secret manifest YAML of type Opaque.
// Values in data are raw bytes; they are base64-encoded on marshal by the
// k8s Secret type. Panics only if yaml.Marshal fails, which cannot happen
// for well-formed Secret values.
func BuildOpaqueSecret(namespace, name string, data map[string][]byte) string {
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
		panic(fmt.Sprintf("BuildOpaqueSecret: %v", err))
	}
	return string(out)
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
		if result != nil && result.ExitCode == 0 {
			return nil
		}

		env.Logger.Info(fmt.Sprintf("creating %s namespace", namespace))
		if _, err := env.Exec.RunChecked(ctx, "oc", "create", "namespace", namespace); err != nil {
			return fmt.Errorf("failed to create %s namespace: %w", namespace, err)
		}
		return nil
	})
}
