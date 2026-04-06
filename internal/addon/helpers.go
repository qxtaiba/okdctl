package addon

import (
	"context"
	"fmt"
	"time"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/retry"
)

// EnsureNamespace checks whether a Kubernetes namespace exists and creates it
// if missing. The operation is retried up to 3 times with exponential backoff
// to handle transient API errors during cluster bootstrap. This is shared
// across addons that need to bootstrap namespaces.
func EnsureNamespace(ctx context.Context, env *Environment, namespace string) error {
	return retry.Do(ctx, 3, 5*time.Second, func() error {
		result, err := env.Exec.Run(ctx, "oc", "get", "namespace", namespace)
		if err == nil && result != nil && result.ExitCode == 0 {
			return nil
		}

		env.Logger.Info(fmt.Sprintf("creating %s namespace", namespace))
		createResult, createErr := env.Exec.Run(ctx, "oc", "create", "namespace", namespace)
		if createErr != nil {
			return utils.WrapError(fmt.Sprintf("failed to create %s namespace", namespace), createErr)
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
