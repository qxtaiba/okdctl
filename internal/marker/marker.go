// Package marker is the schema-versioned JSON file primitive behind the
// cluster record: payloads written atomically at mode 0600 and stamped with
// the cluster name for the reader to check.
package marker

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/qxtaiba/okdctl/internal/system"
)

// ErrSchemaVersion reports a marker written under another schema version.
var ErrSchemaVersion = errors.New("unsupported schema_version")

// Envelope is the header every marker payload embeds; File.Write stamps it
// and File.Read checks SchemaVersion.
type Envelope struct {
	SchemaVersion string `json:"schema_version"`
	ClusterName   string `json:"cluster_name,omitempty"`
}

func (e *Envelope) envelope() *Envelope { return e }

// Payload is a marker payload; satisfied by embedding Envelope.
type Payload interface{ envelope() *Envelope }

// File is one marker file's schema contract.
type File struct {
	Label   string
	Version string
}

// Write stamps the payload's envelope and atomically writes it at mode 0600.
func (f File) Write(path string, p Payload, clusterName string) error {
	e := p.envelope()
	e.SchemaVersion = f.Version
	e.ClusterName = clusterName
	data, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal %s marker: %w", f.Label, err)
	}
	return system.AtomicWrite(path, data, 0o600)
}

// Read decodes the marker at path into p; a missing file wraps
// os.ErrNotExist and another schema version wraps ErrSchemaVersion.
func (f File) Read(path string, p Payload) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s marker: %w", f.Label, err)
	}
	if err := json.Unmarshal(data, p); err != nil {
		return fmt.Errorf("parse %s marker %s: %w", f.Label, path, err)
	}
	if version := p.envelope().SchemaVersion; version != f.Version {
		return fmt.Errorf("%s marker %s: %w %q, want %q", f.Label, path, ErrSchemaVersion, version, f.Version)
	}
	return nil
}
