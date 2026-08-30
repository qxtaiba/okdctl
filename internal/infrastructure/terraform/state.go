package terraform

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
)

// checkStateMajorVersion enforces requiredTerraformMajor against stateFile's
// terraform_version (keep in sync with versions.tf); parse failures are non-fatal.
func checkStateMajorVersion(stateFile string, log *slog.Logger) error {
	const requiredTerraformMajor = 1

	raw, err := os.ReadFile(stateFile)
	if err != nil {
		log.Warn("terraform: state file read failed; skipping major-version preflight",
			"path", stateFile, "err", err)
		return nil
	}
	var state struct {
		TerraformVersion string `json:"terraform_version"`
	}
	if err := json.Unmarshal(raw, &state); err != nil || state.TerraformVersion == "" {
		log.Warn("terraform: state version unreadable; skipping major-version preflight",
			"path", stateFile)
		return nil //nolint:nilerr // parse failure is non-fatal: terraform's own init/plan path surfaces it
	}
	parts := strings.SplitN(state.TerraformVersion, ".", 2)
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		log.Warn("terraform: state version unparseable; skipping major-version preflight",
			"version", state.TerraformVersion)
		return nil //nolint:nilerr // semver parse failure is non-fatal: caller continues to terraform init
	}
	if major != requiredTerraformMajor {
		return &errtypes.ConfigError{
			Msg: fmt.Sprintf(
				"terraform state was written by terraform v%s (major %d); versions.tf requires major %d — upgrade terraform or migrate the state before destroying",
				state.TerraformVersion, major, requiredTerraformMajor,
			),
		}
	}
	return nil
}

// StateHasResource reports whether addr is present via terraform state list
// <addr>. Only exit 1 with empty stdout/stderr means absent; any other non-zero
// exit is a hard error.
func (t *Executor) StateHasResource(ctx context.Context, addr string) (bool, error) {
	result, err := t.exec.RunOutput(ctx, 0, "terraform", "state", "list", addr)
	if err != nil {
		return false, fmt.Errorf("terraform state list %s: %w", addr, err)
	}
	switch {
	case result.ExitCode == 0:
		return true, nil
	case result.ExitCode == 1 && strings.TrimSpace(result.Stdout) == "" && strings.TrimSpace(result.Stderr) == "":
		return false, nil
	default:
		return false, executor.NewExitError(ctx, "terraform state list "+addr, result.ExitCode, result.Stderr)
	}
}
