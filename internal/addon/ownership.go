package addon

import (
	"context"
	"fmt"
	"strings"

	"github.com/qxtaiba/okdctl/internal/executor"
)

// RollbackForNewNamespace permits compensation only when the namespace was absent before install.
func RollbackForNewNamespace(ctx context.Context, env *Environment, namespace string, rollback func(context.Context) error) (func(context.Context) error, error) {
	result, err := env.Exec.Run(ctx, "oc", "get", "namespace", namespace, "--ignore-not-found", "-o", "name")
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 {
		return nil, executor.NewExitError(ctx, "oc get namespace", result.ExitCode, result.Stderr)
	}
	if result.Truncated {
		return nil, fmt.Errorf("inspect namespace ownership: output truncated")
	}
	if strings.TrimSpace(result.Stdout) != "" {
		return nil, nil
	}
	return rollback, nil
}
