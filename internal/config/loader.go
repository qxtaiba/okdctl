package config

import (
	"fmt"
	"os"

	"sigs.k8s.io/yaml"

	"github.com/qxtaiba/okdctl/internal/system"
)

// Loader reads and writes cluster Config YAML files.
type Loader struct{}

// NewLoader returns a Loader suitable for reading okdctl YAML configs.
func NewLoader() *Loader { return &Loader{} }

// LoadFile parses the YAML config at path and returns the merged Config.
// World- or group-writable files are rejected to keep an attacker-writable
// host from influencing deploy behavior. Unknown top-level keys and wrong
// schemaVersion values produce errors rather than silent defaults.
func (l *Loader) LoadFile(path string) (*Config, error) {
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
	if err := yaml.UnmarshalStrict(data, cfg); err != nil {
		return nil, fmt.Errorf("error parsing config: %w", err)
	}
	if cfg.SchemaVersion == "" {
		return nil, fmt.Errorf("config file %s missing required schemaVersion (expected %q)", path, SchemaVersionV1)
	}
	if cfg.SchemaVersion != SchemaVersionV1 {
		return nil, fmt.Errorf("config file %s has unsupported schemaVersion %q (expected %q)", path, cfg.SchemaVersion, SchemaVersionV1)
	}
	return cfg, nil
}

// Save writes cfg to path with 0o600 perms via AtomicWrite. SchemaVersion is
// set to SchemaVersionV1 when empty.
func (l *Loader) Save(cfg *Config, path string) error {
	if cfg.SchemaVersion == "" {
		cfg.SchemaVersion = SchemaVersionV1
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return system.AtomicWrite(path, data, 0o600)
}
