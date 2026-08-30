package config

import (
	"fmt"
	"os"

	"sigs.k8s.io/yaml"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/netutil"
	"github.com/qxtaiba/okdctl/internal/system"
)

// Loader reads and writes cluster Config YAML files, kept as a struct (not
// package funcs) so a future stateful Loader can land without breaking call sites.
type Loader struct{}

// NewLoader returns a Loader for okdctl YAML configs.
func NewLoader() *Loader { return &Loader{} }

// LoadFile parses the YAML config at path and returns the merged Config.
// World/group-writable files are rejected, and unknown keys or the wrong
// schemaVersion error instead of silently defaulting.
func (l *Loader) LoadFile(path string) (*Config, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, &errtypes.ConfigError{Msg: fmt.Sprintf("stat config file %s", path), Err: err}
	}
	if perm := fi.Mode().Perm(); perm&0o022 != 0 {
		return nil, &errtypes.AuthError{
			Msg: fmt.Sprintf("config file %s has insecure permissions %#o; run 'chmod go-w %s' to fix", path, perm, path),
			Err: os.ErrPermission,
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &errtypes.ConfigError{Msg: fmt.Sprintf("read config file %s", path), Err: err}
	}

	if err := checkSchemaVersion(data, path); err != nil {
		return nil, err
	}

	cfg := DefaultConfig()
	if err := yaml.UnmarshalStrict(data, cfg); err != nil {
		return nil, &errtypes.ConfigError{Msg: "parse config", Err: err}
	}
	deriveStaticNetmask(cfg)
	return cfg, nil
}

// checkSchemaVersion runs before the strict unmarshal so a bad schema fails
// with a clear version error, not an opaque unknown-field one.
func checkSchemaVersion(data []byte, path string) error {
	var probe struct {
		SchemaVersion string `json:"schemaVersion"`
	}
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return &errtypes.ConfigError{Msg: "parse config", Err: err}
	}
	switch probe.SchemaVersion {
	case SchemaVersionCurrent:
		return nil
	case "":
		return &errtypes.ConfigError{Msg: fmt.Sprintf("config file %s missing required schemaVersion (expected %q)", path, SchemaVersionCurrent)}
	default:
		return &errtypes.ConfigError{Msg: fmt.Sprintf("config file %s has unsupported schemaVersion %q (expected %q)", path, probe.SchemaVersion, SchemaVersionCurrent)}
	}
}

// deriveStaticNetmask overwrites StaticIP.Netmask with MachineCIDR's dotted
// form so hand-edits can't desync them; invalid/IPv6 CIDRs are left for validators.
func deriveStaticNetmask(cfg *Config) {
	if netmask, err := netutil.CIDRToNetmask(cfg.Networking.MachineCIDR); err == nil {
		cfg.Networking.StaticIP.Netmask = netmask
	}
}

// Save writes cfg to path with 0o600 perms via AtomicWrite. SchemaVersion is
// set to SchemaVersionCurrent when empty.
func (l *Loader) Save(cfg *Config, path string) error {
	if cfg.SchemaVersion == "" {
		cfg.SchemaVersion = SchemaVersionCurrent
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return &errtypes.ConfigError{Msg: "marshal config", Err: err}
	}
	if err := system.AtomicWrite(path, data, 0o600); err != nil {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("write config to %s", path), Err: err}
	}
	return nil
}
