package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

const (
	ConfigFileName = "openshitctl"
	ConfigFileType = "yaml"
	ConfigDirName  = ".openshitctl"
)

type Loader struct {
	viper *viper.Viper
}

func NewLoader() *Loader {
	v := viper.New()
	v.SetConfigName(ConfigFileName)
	v.SetConfigType(ConfigFileType)

	v.AddConfigPath(".")
	if home, err := os.UserHomeDir(); err == nil {
		v.AddConfigPath(filepath.Join(home, ConfigDirName))
	}
	v.AddConfigPath("/etc/openshitctl")

	// AutomaticEnv binds HOMELAB_* env vars into the config tree via mapstructure.
	// Credential fields (Password, APIToken) carry `mapstructure:"-"` so they are
	// never populated from the environment this way. Credentials flow exclusively
	// through internal/credentials from the .env file.
	v.SetEnvPrefix("HOMELAB")
	v.AutomaticEnv()

	return &Loader{viper: v}
}

// Load returns the default config if no config file is found.
func (l *Loader) Load() (*Config, error) {
	cfg := DefaultConfig()

	if err := l.viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return cfg, nil
		}
		return nil, fmt.Errorf("error reading config file %q: %w", l.viper.ConfigFileUsed(), err)
	}

	if err := l.viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("error parsing config: %w", err)
	}

	migrateDeprecatedFields(cfg)
	return cfg, nil
}

func (l *Loader) LoadFile(path string) (*Config, error) {
	// Reject world- or group-writable config files. The YAML is not
	// supposed to carry secrets, but if a bug ever leaks one we don't
	// want an attacker-writable file on the deploy host to matter.
	fi, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("error stating config file %s: %w", path, err)
	}
	if perm := fi.Mode().Perm(); perm&0022 != 0 {
		return nil, fmt.Errorf("config file %s has insecure permissions %#o; run 'chmod go-w %s' to fix: %w", path, perm, path, os.ErrPermission)
	}

	l.viper.SetConfigFile(path)

	if err := l.viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("error reading config file %s: %w", path, err)
	}

	cfg := DefaultConfig()
	if err := l.viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("error parsing config: %w", err)
	}

	migrateDeprecatedFields(cfg)
	return cfg, nil
}

type LoadResult struct {
	Config     *Config
	Validation *ValidationResult
}

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

// MustLoadFile returns an error if validation fails.
func (l *Loader) MustLoadFile(path string) (*Config, error) {
	result, err := l.LoadFileWithValidation(path)
	if err != nil {
		return nil, err
	}

	if !result.Validation.IsValid() {
		return nil, fmt.Errorf("configuration validation failed: %s", result.Validation.Error())
	}

	return result.Config, nil
}

func (l *Loader) Save(cfg *Config, path string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	return system.AtomicWrite(path, data, 0600)
}

// migrateDeprecatedFields promotes old config fields to their replacements so
// the wizard, review step, and terraform var generation all see consistent
// values immediately after load.
func migrateDeprecatedFields(cfg *Config) {
	if cfg.Disks.WorkerDataSizeGB == 0 && cfg.Disks.DataSizeGB > 0 {
		cfg.Disks.WorkerDataSizeGB = cfg.Disks.DataSizeGB
	}
}
