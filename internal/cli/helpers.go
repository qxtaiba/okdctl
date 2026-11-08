package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

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
	if configFile != "okdctl.yaml" {
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

func resolveProjectRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(wd)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		// ErrNotExist here is benign (path not created yet); any other error
		// must not be swallowed.
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("resolve project root symlinks: %w", err)
		}
		return abs, nil
	}
	return resolved, nil
}

// resolveWorkspaceRoot resolves cwd without requiring project markers; init
// uses it directly since init is what creates the markers.
func resolveWorkspaceRoot() (string, error) {
	root, err := resolveProjectRoot()
	if err != nil {
		return "", fmt.Errorf("resolve project root: %w", err)
	}
	if root == "" {
		return "", fmt.Errorf("project root resolved to empty path")
	}
	return root, nil
}

func resolveProjectRootOrDie() (string, error) {
	root, err := resolveWorkspaceRoot()
	if err != nil {
		return "", err
	}
	if !hasPrimaryMarker(root) {
		return "", &errtypes.ConfigError{
			Msg: fmt.Sprintf("no cluster in %s (checked okdctl.yaml, okdctl.env); run 'okdctl init' first", root),
			Err: errtypes.ErrConfigMissing,
		}
	}
	return root, nil
}

// hasPrimaryMarker reports whether root has the config file or okdctl.env; both
// are written exclusively by okdctl.
func hasPrimaryMarker(root string) bool {
	return slices.ContainsFunc([]string{filepath.Base(cfgFile), "okdctl.env"}, func(name string) bool {
		_, err := os.Stat(filepath.Join(root, name))
		return err == nil
	})
}
