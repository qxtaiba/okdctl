package config

import (
	"os"
	"path/filepath"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

const (
	ConfigFileName = "openshitctl"
	ConfigFileType = "yaml"
	ConfigDirName  = ".openshitctl"
)

// Loader handles loading and saving configuration.
type Loader struct {
	viper *viper.Viper
}

// NewLoader creates a new configuration loader.
func NewLoader() *Loader {
	v := viper.New()
	v.SetConfigName(ConfigFileName)
	v.SetConfigType(ConfigFileType)

	// Search paths in priority order
	v.AddConfigPath(".")
	if home, err := os.UserHomeDir(); err == nil {
		v.AddConfigPath(filepath.Join(home, ConfigDirName))
	}
	v.AddConfigPath("/etc/openshitctl")

	v.SetEnvPrefix("HOMELAB")
	v.AutomaticEnv()

	return &Loader{viper: v}
}

// Load reads configuration from file and environment variables.
// Returns the default config if no config file is found.
func (l *Loader) Load() (*Config, error) {
	cfg := DefaultConfig()

	if err := l.viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return cfg, nil
		}
		return nil, utils.WrapErrorf(err, "error reading config file %q", l.viper.ConfigFileUsed())
	}

	if err := l.viper.Unmarshal(cfg); err != nil {
		return nil, utils.WrapError("error parsing config", err)
	}

	return cfg, nil
}

// LoadFile reads configuration from a specific file path.
func (l *Loader) LoadFile(path string) (*Config, error) {
	l.viper.SetConfigFile(path)

	if err := l.viper.ReadInConfig(); err != nil {
		return nil, utils.WrapErrorf(err, "error reading config file %s", path)
	}

	cfg := DefaultConfig()
	if err := l.viper.Unmarshal(cfg); err != nil {
		return nil, utils.WrapError("error parsing config", err)
	}

	return cfg, nil
}

// LoadResult contains the loaded config and validation issues.
type LoadResult struct {
	Config     *Config
	Validation *ValidationResult
}

// LoadFileWithValidation reads and validates configuration from a file path.
func (l *Loader) LoadFileWithValidation(path string) (*LoadResult, error) {
	cfg, err := l.LoadFile(path)
	if err != nil {
		return nil, err
	}

	validation := cfg.Validate()
	return &LoadResult{
		Config:     cfg,
		Validation: validation,
	}, nil
}

// MustLoadFile reads configuration and returns an error if validation fails.
func (l *Loader) MustLoadFile(path string) (*Config, error) {
	result, err := l.LoadFileWithValidation(path)
	if err != nil {
		return nil, err
	}

	if !result.Validation.IsValid() {
		return nil, utils.WrapErrorf(nil, "configuration validation failed: %s", result.Validation.Error())
	}

	return result.Config, nil
}

// Save writes the configuration to a file.
func (l *Loader) Save(cfg *Config, path string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	return system.AtomicWrite(path, data, 0644)
}

// Set updates a configuration value by key path.
func (l *Loader) Set(key string, value interface{}) {
	l.viper.Set(key, value)
}

// Get retrieves a configuration value by key path.
func (l *Loader) Get(key string) interface{} {
	return l.viper.Get(key)
}

