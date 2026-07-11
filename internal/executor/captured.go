package executor

import (
	"context"
	"fmt"
)

// RunCaptured runs bin with args on a scratch Executor, discarding stdout
// and returning an error on non-zero exit. Cancel sends cancelSignal
// (SIGTERM by default — see WithCancelSignal); env is filtered through
// DefaultEnvAllowlist so caller shell tokens never reach privileged
// children. Non-zero exit and launch failure both return an error;
// callers that need stdout should use OutputCaptured.
func RunCaptured(ctx context.Context, bin string, args ...string) error {
	_, err := New().RunDiscardChecked(ctx, bin, args...)
	return err
}

// OutputCaptured runs bin with args on a scratch Executor and returns full
// stdout, capped at runOutputMaxBytes. A capacity overrun fails loudly via
// *ExitError instead of silently truncating, since OutputCaptured callers
// depend on a complete stream. Shares cancelSignal, WaitDelay, and
// DefaultEnvAllowlist filtering with RunCaptured.
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
