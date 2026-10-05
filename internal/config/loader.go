package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	yamlv2 "go.yaml.in/yaml/v2"
	"sigs.k8s.io/yaml"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/system"
)

// Loader reads and writes cluster Config YAML files, kept as a struct (not
// package funcs) so a future stateful Loader can land without breaking call sites.
type Loader struct{}

// NewLoader returns a Loader for okdctl YAML configs.
func NewLoader() *Loader { return &Loader{} }

// LoadFile parses the YAML config at path and returns it as authored —
// omitted bootstrap/worker fields stay zero rather than being filled in, so
// a later Save never materializes a value the operator did not write;
// callers needing resolved topology values must call Effective. The static
// netmask is the one field derived eagerly here (see DeriveStaticNetmask).
// World/group-writable files are rejected, and unknown keys, a second YAML
// document, or the wrong schemaVersion error instead of silently defaulting.
func (l *Loader) LoadFile(path string) (*Config, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, &errtypes.ConfigError{Msg: err.Error(), Err: err}
	}
	if perm := fi.Mode().Perm(); perm&0o022 != 0 {
		return nil, &errtypes.AuthError{
			Msg: fmt.Sprintf("config file %s has insecure permissions %#o; run 'chmod go-w %s' to fix", path, perm, path),
			Err: os.ErrPermission,
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &errtypes.ConfigError{Msg: err.Error(), Err: err}
	}

	if envFileAssignment.Match(data) {
		return nil, &errtypes.ConfigError{Msg: fmt.Sprintf("config file %s is a credentials env file (it assigns PROXMOX_VE_* variables), not a YAML config", path)}
	}
	if err := rejectMultipleDocuments(data, path); err != nil {
		return nil, err
	}
	if err := checkSchemaVersion(data, path); err != nil {
		return nil, err
	}

	cfg := fileDefaults()
	if err := yaml.UnmarshalStrict(data, cfg); err != nil {
		return nil, parseError(path, err)
	}
	_ = DeriveStaticNetmask(cfg) // invalid/IPv6 CIDR is left for validators
	return cfg, nil
}

// envFileAssignment matches a line of okdctl.env; decoder errors can quote
// the file, so one passed as the config is refused before any decoding.
var envFileAssignment = regexp.MustCompile(`(?m)^[ \t]*PROXMOX_VE_[A-Z_]+[ \t]*=`)

func parseError(path string, err error) error {
	cause := err
	for inner := errors.Unwrap(cause); inner != nil; inner = errors.Unwrap(cause) {
		cause = inner
	}
	detail := strings.Join(strings.Fields(strings.TrimPrefix(cause.Error(), "json: ")), " ")
	return &errtypes.ConfigError{Msg: fmt.Sprintf("parse config file %s: %s", path, detail), Err: err}
}

// rejectMultipleDocuments exists because yaml.Unmarshal reads only the first
// document and would silently drop everything after a "---" separator.
func rejectMultipleDocuments(data []byte, path string) error {
	dec := yamlv2.NewDecoder(bytes.NewReader(data))
	var doc any
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return parseError(path, err)
	}
	err := dec.Decode(&doc)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return parseError(path, err)
	}
	return &errtypes.ConfigError{Msg: fmt.Sprintf(`config file %s contains more than one YAML document; merge them into one and remove the "---" separator`, path)}
}

// checkSchemaVersion runs before the strict unmarshal so a bad schema fails
// with a clear version error, not an opaque unknown-field one.
func checkSchemaVersion(data []byte, path string) error {
	var probe struct {
		SchemaVersion string `json:"schemaVersion"`
	}
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return parseError(path, err)
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
