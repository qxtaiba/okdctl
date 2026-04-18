package config

import (
	"fmt"
	"os"

	"sigs.k8s.io/yaml"

	"github.com/qxtaiba/okdctl/internal/system"
)

type Loader struct{}

func NewLoader() *Loader { return &Loader{} }

func (l *Loader) LoadFile(path string) (*Config, error) {
	// Reject world- or group-writable config files. The YAML is not
	// supposed to carry secrets, but if a bug ever leaks one we don't
	// want an attacker-writable file on the deploy host to matter.
	fi, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("error stating config file %s: %w", path, err)
	}
	if perm := fi.Mode().Perm(); perm&0o022 != 0 {
		return nil, fmt.Errorf("config file %s has insecure permissions %#o; run 'chmod go-w %s' to fix: %w", path, perm, path, os.ErrPermission)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading config file %s: %w", path, err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("error parsing config: %w", err)
	}
	return cfg, nil
}

func (l *Loader) Save(cfg *Config, path string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return system.AtomicWrite(path, data, 0o600)
}
