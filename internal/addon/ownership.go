package addon

import (
	"context"
	"fmt"
	"strings"

	"github.com/qxtaiba/okdctl/internal/executor"
)

// OwnershipLabel marks a namespace EnsureNamespace created, so rollback and
// uninstall decide ownership from that label instead of bare existence —
// existence alone degrades across retries once a failed attempt leaves
// debris behind.
const OwnershipLabel = "okdctl.io/managed-by"

// ownershipLabelValue is OwnershipLabel's value on namespaces okdctl creates.
const ownershipLabelValue = "okdctl"

// NamespaceOwnedByOkdctl reports whether namespace exists and carries
// OwnershipLabel, i.e. was created by EnsureNamespace rather than
// pre-existing.
func NamespaceOwnedByOkdctl(ctx context.Context, env *Environment, namespace string) (bool, error) {
	result, err := env.Exec.Run(ctx, "oc", "get", "namespace", namespace,
		"-l", OwnershipLabel+"="+ownershipLabelValue, "--ignore-not-found", "-o", "name")
	if err != nil {
		return false, err
	}
	if result.ExitCode != 0 {
		return false, executor.NewExitError(ctx, "oc get namespace", result.ExitCode, result.Stderr)
	}
	if result.Truncated {
		return false, fmt.Errorf("inspect namespace ownership: output truncated")
	}
	return strings.TrimSpace(result.Stdout) != "", nil
}

// RollbackForNewNamespace permits compensation when the namespace is absent
// (install is about to create and own it) or already carries OwnershipLabel
// (debris okdctl owns from a prior attempt); it denies compensation for any
// other pre-existing namespace so a foreign resource is never deleted.
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
	if strings.TrimSpace(result.Stdout) == "" {
		return rollback, nil
	}
	owned, err := NamespaceOwnedByOkdctl(ctx, env, namespace)
	if err != nil {
		return nil, err
	}
	if !owned {
		return nil, nil
	}
	return rollback, nil
}
