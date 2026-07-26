package debugbundle

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/qxtaiba/okdctl/internal/executor"
)

// collectDoctorOutput re-execs the current binary with the `doctor`
// subcommand and returns stdout (the --output json document) and stderr
// (json-formatted warn logs) separately, so a warn line can never interleave
// into the archived JSON. Non-zero exit (failing preflight checks) is not
// treated as an error — the output is still useful for a support engineer.
func collectDoctorOutput(ctx context.Context) (stdout, stderr []byte, err error) {
	self, err := os.Executable()
	if err != nil {
		return nil, nil, fmt.Errorf("resolve binary path: %w", err)
	}
	var outBuf, errBuf bytes.Buffer
	cmd := exec.CommandContext(ctx, self, "doctor", "--output", "json", "--log-format", "json", "--log-level", "warn")
	// Same allowlist as the sudo re-exec in cli/elevation.go: the child is
	// okdctl itself, but unrelated shell tokens still have no business in it.
	cmd.Env = executor.FilterParentEnv(executor.DefaultEnvAllowlist)
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	_ = cmd.Run()
	return outBuf.Bytes(), errBuf.Bytes(), nil
}
