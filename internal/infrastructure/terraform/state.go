package terraform

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

// checkStateMajorVersion reads stateFile, extracts .terraform_version, and
// returns *errtypes.ConfigError when the parsed major component falls outside
// [stateMajorMin, stateMajorMax]. Bounds match
// infrastructure/terraform/environments/production/versions.tf
// (required_version = ">= 1.10, < 2.0"). Update both constants if the
// constraint ever crosses a major boundary. Parsing failures are non-fatal:
// they are logged and the caller continues so terraform itself can surface
// the issue.
func checkStateMajorVersion(stateFile string, log *slog.Logger) error {
	const stateMajorMin, stateMajorMax = 1, 1

	raw, err := os.ReadFile(stateFile)
	if err != nil {
		log.Warn("terraform: state file read failed; skipping major-version preflight",
			"file", stateFile, "err", err)
		return nil
	}
	var state struct {
		TerraformVersion string `json:"terraform_version"`
	}
	if err := json.Unmarshal(raw, &state); err != nil || state.TerraformVersion == "" {
		log.Warn("terraform: state version unreadable; skipping major-version preflight",
			"file", stateFile)
		return nil //nolint:nilerr // parse failure is non-fatal: terraform's own init/plan path surfaces it
	}
	parts := strings.SplitN(state.TerraformVersion, ".", 2)
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		log.Warn("terraform: state version unparseable; skipping major-version preflight",
			"version", state.TerraformVersion)
		return nil //nolint:nilerr // semver parse failure is non-fatal: caller continues to terraform init
	}
	if major < stateMajorMin || major > stateMajorMax {
		return &errtypes.ConfigError{
			Msg: fmt.Sprintf(
				"terraform state was written by terraform v%s (major %d); versions.tf requires major %d — upgrade terraform or migrate the state before destroying",
				state.TerraformVersion, major, stateMajorMin,
			),
		}
	}
	return nil
}
