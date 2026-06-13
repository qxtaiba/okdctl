package cluster

import (
	"context"
	"strings"

	"github.com/qxtaiba/okdctl/internal/executor"
)

// RawGet runs `<cli> get --raw <path>` and returns trimmed stdout.
// A non-zero exit is returned as an *executor.ExitError.
func (c *Client) RawGet(ctx context.Context, path string) (string, error) {
	result, err := c.runOutput(ctx, "get", "--raw", path)
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return "", executor.NewExitError(ctx, c.CLI+" get --raw", result.ExitCode, strings.TrimSpace(result.Stderr))
	}
	return strings.TrimSpace(result.Stdout), nil
}

// GetJSON runs `<cli> <args...>` with full stdout buffering and returns trimmed
// stdout, a truncated flag, and any execution error. A non-zero exit is returned
// as an *executor.ExitError. Callers must check truncated before unmarshalling to
// avoid silently processing a capped JSON payload.
func (c *Client) GetJSON(ctx context.Context, args ...string) (stdout string, truncated bool, err error) {
	result, runErr := c.runOutput(ctx, args...)
	if runErr != nil {
		return "", false, runErr
	}
	if result.ExitCode != 0 {
		return "", false, executor.NewExitError(ctx, c.CLI+" "+subcommand(args), result.ExitCode, strings.TrimSpace(result.Stderr))
	}
	return strings.TrimSpace(result.Stdout), result.Truncated, nil
}
