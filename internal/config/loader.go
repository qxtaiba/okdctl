package config

import (
	"fmt"
	"os"

	"sigs.k8s.io/yaml"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/netutil"
	"github.com/qxtaiba/okdctl/internal/system"
)

// Loader reads and writes cluster Config YAML files.
// It is intentionally stateless today; the struct shape is the
// canonical surface so a future stateful Loader (e.g. caching parsed
// configs across CLI subcommands, decryption keyring) can land without
// breaking call-site shapes. Do not collapse to package-level functions.
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
		return nil, &errtypes.ConfigError{Msg: fmt.Sprintf("error stating config file %s", path), Err: err}
	}
	if perm := fi.Mode().Perm(); perm&0o022 != 0 {
		return nil, &errtypes.AuthError{
			Msg: fmt.Sprintf("config file %s has insecure permissions %#o; run 'chmod go-w %s' to fix", path, perm, path),
			Err: os.ErrPermission,
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &errtypes.ConfigError{Msg: fmt.Sprintf("error reading config file %s", path), Err: err}
	}

	if err := checkSchemaVersion(data, path); err != nil {
		return nil, err
	}

	cfg := DefaultConfig()
	if err := yaml.UnmarshalStrict(data, cfg); err != nil {
		return nil, &errtypes.ConfigError{Msg: "error parsing config", Err: err}
	}
	deriveStaticNetmask(cfg)
	return cfg, nil
}

// checkSchemaVersion gates on schemaVersion before the strict unmarshal so a
// stale config fails with a migration message naming the renamed keys, not
// an opaque unknown-field error.
func checkSchemaVersion(data []byte, path string) error {
	var probe struct {
		SchemaVersion string `json:"schemaVersion"`
	}
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return &errtypes.ConfigError{Msg: "error parsing config", Err: err}
	}
	switch probe.SchemaVersion {
	case SchemaVersionCurrent:
		return nil
	case "":
		return &errtypes.ConfigError{Msg: fmt.Sprintf("config file %s missing required schemaVersion (expected %q)", path, SchemaVersionCurrent)}
	case SchemaVersionV1:
		return &errtypes.ConfigError{Msg: fmt.Sprintf(
			"config file %s uses schemaVersion %q; current is %q — rename provider.proxmox.master_nodes to control_plane_nodes, disks.master_data_size_gb to control_plane_data_size_gb, topology.*.memory to memory_mb, topology.*.disk to disk_gb, then set schemaVersion: %q",
			path, SchemaVersionV1, SchemaVersionCurrent, SchemaVersionCurrent)}
	default:
		return &errtypes.ConfigError{Msg: fmt.Sprintf("config file %s has unsupported schemaVersion %q (expected %q)", path, probe.SchemaVersion, SchemaVersionCurrent)}
	}
}

// deriveStaticNetmask overwrites StaticIP.Netmask with the dotted form of
// MachineCIDR so the two subnet encodings cannot desync via a hand-edit.
// Invalid or IPv6 CIDRs are left for the networking validators to report.
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
		return &errtypes.ConfigError{Msg: "failed to marshal config", Err: err}
	}
	if err := system.AtomicWrite(path, data, 0o600); err != nil {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("failed to write config to %s", path), Err: err}
	}
	return nil
}
