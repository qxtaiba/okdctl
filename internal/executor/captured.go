package executor

import (
	"context"
	"fmt"
)

// RunCaptured runs bin with args on a scratch Executor, discarding stdout
// and erroring on non-zero exit or launch failure; env is filtered through
// DefaultEnvAllowlist so caller tokens never reach the child.
func RunCaptured(ctx context.Context, bin string, args ...string) error {
	_, err := New().RunDiscardChecked(ctx, bin, args...)
	return err
}

// OutputCaptured runs bin with args on a scratch Executor and returns full
// stdout capped at runOutputMaxBytes, failing via *ExitError on overrun
// rather than silently truncating.
func OutputCaptured(ctx context.Context, bin string, args ...string) ([]byte, error) {
	result, err := New().RunOutputChecked(ctx, 0, bin, args...)
	if err != nil {
		return nil, err
	}
	if result.Truncated {
		return nil, NewExitError(ctx, bin, result.ExitCode,
			fmt.Sprintf("output exceeded %d bytes", runOutputMaxBytes))
	}
	return []byte(result.Stdout), nil
}
