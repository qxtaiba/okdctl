package debugbundle

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/qxtaiba/okdctl/internal/executor"
)

// collectDoctorOutput re-execs the binary as `doctor`, separating stdout
// (json) from stderr; non-zero exit is not an error.
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
	_ = cmd.Run()
	return outBuf.Bytes(), errBuf.Bytes(), nil
}
