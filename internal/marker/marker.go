// Package marker is the schema-versioned JSON file primitive behind the
// cluster record: payloads written atomically at mode 0600 and stamped with
// the cluster name for the reader to check.
package marker

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/system"
)

// Envelope is the header common to every marker payload. Concrete payload
// types embed it; File.Write stamps it and File.Read validates SchemaVersion.
type Envelope struct {
	SchemaVersion string    `json:"schema_version"`
	RunID         string    `json:"run_id"`
	Timestamp     time.Time `json:"timestamp"`
	ClusterName   string    `json:"cluster_name,omitempty"`
}

func (e *Envelope) envelope() *Envelope { return e }

// Payload is a marker payload; satisfied by embedding Envelope.
type Payload interface{ envelope() *Envelope }

// File is one marker file's schema contract.
type File struct {
	Label   string
	Version string
}

// Write stamps the payload's envelope (current schema version, run id,
// cluster name, UTC timestamp) and atomically writes it at mode 0600.
func (f File) Write(path string, p Payload, runID, clusterName string) error {
	e := p.envelope()
	e.SchemaVersion = f.Version
	e.RunID = runID
	e.ClusterName = clusterName
	e.Timestamp = time.Now().UTC()
	data, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal %s marker: %w", f.Label, err)
	}
	return system.AtomicWrite(path, data, 0o600)
}

// Read decodes the marker at path into p. A missing file and an
// unrecognized schema version both read as found=false with a nil error;
// unreadable or unparseable files return an error.
func (f File) Read(path string, p Payload) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s marker: %w", f.Label, err)
	}
	if err := json.Unmarshal(data, p); err != nil {
		return false, fmt.Errorf("parse %s marker: %w", f.Label, err)
	}
	version := p.envelope().SchemaVersion
	if version == f.Version {
		return true, nil
	}
	logutil.Warn("ignoring marker with unknown schema_version",
		logutil.LF("marker", f.Label),
		logutil.LF("schema_version", version),
		logutil.LF("expected", f.Version))
	return false, nil
}

// Clear removes the marker; a missing file is success.
func (f File) Clear(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s marker: %w", f.Label, err)
	}
	return nil
}
