package debugbundle

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
)

// collectDoctorOutput re-execs the current binary with the `doctor` subcommand
// and returns its combined stdout+stderr. Non-zero exit (failing preflight
// checks) is not treated as an error — the output is still useful for a
// support engineer.
func collectDoctorOutput(ctx context.Context) ([]byte, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve binary path: %w", err)
	}
	var buf bytes.Buffer
	cmd := exec.CommandContext(ctx, self, "doctor", "--output", "json", "--log-format", "json", "--log-level", "warn")
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	_ = cmd.Run()
	return buf.Bytes(), nil
}
