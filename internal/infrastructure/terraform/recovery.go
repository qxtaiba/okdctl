package terraform

import (
	"context"
	"fmt"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

// RecoverySource supplies the state snapshot and lock diagnostics for a mutation.
type RecoverySource interface {
	SnapshotState(context.Context) (string, error)
	WithLockHint(error) error
}

// WithStateRecovery snapshots state before mutation and preserves backup location and error identity.
func WithStateRecovery(ctx context.Context, source RecoverySource, operation string, mutate func() error) error {
	backup, err := source.SnapshotState(ctx)
	if err != nil {
		return &errtypes.ClusterError{Msg: operation + ": snapshot state", Err: err}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := mutate(); err != nil {
		message := operation
		if backup != "" {
			message = fmt.Sprintf("%s (state backup: %s)", operation, backup)
		}
		return source.WithLockHint(&errtypes.ClusterError{Msg: message, Err: err})
	}
	return nil
}
