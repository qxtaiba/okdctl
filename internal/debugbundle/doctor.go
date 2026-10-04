package debugbundle

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/qxtaiba/okdctl/internal/executor"
)

// collectDoctorOutput re-execs the binary as `doctor`, separating stdout
// (json) from stderr; doctor health-result exits remain usable output.
func collectDoctorOutput(ctx context.Context) (stdout, stderr []byte, err error) {
	self, err := os.Executable()
	if err != nil {
		return nil, nil, fmt.Errorf("resolve binary path: %w", err)
	}
	var outBuf, errBuf bytes.Buffer
	cmd := exec.CommandContext(ctx, self, "doctor", "--output", "json", "--log-format", "json", "--log-level", "warn")
	// Same allowlist as the sudo re-exec in cli/elevation.go — the child is
	// okdctl, but stays scoped.
	cmd.Env = executor.FilterParentEnv(executor.DefaultEnvAllowlist)
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	runErr := cmd.Run()
	if ctx.Err() != nil {
		return outBuf.Bytes(), errBuf.Bytes(), fmt.Errorf("run doctor: %w", ctx.Err())
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) && (exitErr.ExitCode() == 2 || exitErr.ExitCode() == 6) {
		runErr = nil
	}
	if runErr != nil {
		return outBuf.Bytes(), errBuf.Bytes(), fmt.Errorf("run doctor: %w", runErr)
	}
	return outBuf.Bytes(), errBuf.Bytes(), nil
}
