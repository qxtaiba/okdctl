package addon

import (
	"context"
	"fmt"
	"time"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/retry"
)

const (
	DefaultRetryCount   = 3
	DefaultRetryBackoff = 5 * time.Second
)

// RetryDefault wraps retry.Do with the standard addon retry policy.
func RetryDefault(ctx context.Context, fn func() error) error {
	return retry.Do(ctx, DefaultRetryCount, DefaultRetryBackoff, fn)
}

// EnsureNamespace checks whether a Kubernetes namespace exists and creates it
// if missing, using the default addon retry policy.
func EnsureNamespace(ctx context.Context, env *Environment, namespace string) error {
	return RetryDefault(ctx, func() error {
		result, err := env.Exec.Run(ctx, "oc", "get", "namespace", namespace)
		if err == nil && result != nil && result.ExitCode == 0 {
			return nil
		}

		env.Logger.Info(fmt.Sprintf("creating %s namespace", namespace))
		createResult, createErr := env.Exec.Run(ctx, "oc", "create", "namespace", namespace)
		if createErr != nil {
			return fmt.Errorf("failed to create %s namespace: %w", namespace, createErr)
		}
		if createResult == nil || createResult.ExitCode != 0 {
			stderr := ""
			if createResult != nil {
				stderr = createResult.Stderr
			}
			return fmt.Errorf("failed to create %s namespace: %s", namespace, stderr)
		}
		return nil
	})
}
