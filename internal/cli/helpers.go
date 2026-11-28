package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/render"
)

func loadConfig(configFile string) (*config.Config, error) {
	loader := config.NewLoader()
	cfg, err := loader.LoadFile(configFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errConfigNotFound(configFile)
		}
		cause := err.Error()
		if d, ok := errtypes.Describe(err); ok {
			cause = d.Message
		}
		return nil, &errtypes.ConfigError{Msg: "load configuration: " + cause, Err: err}
	}
	return cfg, nil
}

// errConfigNotFound builds the family error every command uses when its
// config file is missing, so the message never drifts between them.
func errConfigNotFound(configFile string) error {
	hint := "run 'okdctl init' to create it"
	if configFile != defaultConfigFile {
		hint = fmt.Sprintf("run 'okdctl init --output-file %s' to create it", configFile)
	}
	configErr := &errtypes.ConfigError{Msg: "configuration file not found: " + configFile, Err: errtypes.ErrConfigMissing}
	return configErr.WithHint(hint)
}

func validateConfig(cfg *config.Config, w io.Writer) *config.ValidationResult {
	result := cfg.Validate()
	if !result.IsValid() {
		fmt.Fprintln(w, render.ValidationSummary(result))
	}
	return result
}

// workspaceRoot returns the directory holding configFile; the record, lock,
// log and kubeconfig of every command live under it.
func workspaceRoot(configFile string) (string, error) {
	abs, err := filepath.Abs(configFile)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}
	return filepath.Dir(abs), nil
}
