// Package marker implements the on-disk resume-marker primitive shared by
// the deploy engine (.okdctl-deploy-state.json) and node ops
// (.okdctl-node-op.json): schema-versioned JSON payloads written atomically
// at mode 0600 and gated on load by a cluster-name guard. Payload semantics
// stay with the owning package; only write/read/guard mechanics live here.
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

// StaleAfter is the age past which a marker is presumed abandoned debris.
const StaleAfter = 7 * 24 * time.Hour

// Envelope is the header common to every marker payload. Concrete payload
// types embed it; File.Write stamps it, File.Read validates SchemaVersion,
// and File.Trusted keys trust on ClusterName.
type Envelope struct {
	SchemaVersion string    `json:"schema_version"`
	RunID         string    `json:"run_id"`
	Timestamp     time.Time `json:"timestamp"`
	ClusterName   string    `json:"cluster_name,omitempty"`
}

func (e *Envelope) envelope() *Envelope { return e }

// Age reports how long ago the marker was written.
func (e *Envelope) Age() time.Duration { return time.Since(e.Timestamp) }

// Stale reports whether the marker is older than StaleAfter.
func (e *Envelope) Stale() bool { return e.Age() >= StaleAfter }

// Payload is a marker payload; satisfied by embedding Envelope.
type Payload interface{ envelope() *Envelope }

// File is one marker file's schema contract. Migrate, when non-nil, maps a
// recognized older schema version's already-decoded payload onto the current
// vocabulary in place and reports whether it recognized the version;
// unrecognized versions are warned about and read as absent.
type File struct {
	Label   string
	Version string
	Migrate func(fromVersion string, p Payload) bool
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
	if f.Migrate != nil && f.Migrate(version, p) {
		return true, nil
	}
	logutil.Warn("ignoring marker with unknown schema_version",
		logutil.LF("marker", f.Label),
		logutil.LF("schema_version", version),
		logutil.LF("expected", f.Version))
	return false, nil
}

// Trusted applies the cluster-name guard. Markers gate resume decisions, so
// a marker must positively identify the current cluster before it is
// trusted: an empty or mismatching cluster name is warned about and
// rejected.
func (f File) Trusted(p Payload, clusterName string) bool {
	e := p.envelope()
	switch {
	case e.ClusterName == "":
		logutil.Warn("marker has no cluster name; treating as absent",
			logutil.LF("marker", f.Label),
			logutil.LF("current_cluster", clusterName))
		return false
	case e.ClusterName != clusterName:
		logutil.Warn("marker is from a different cluster, ignoring",
			logutil.LF("marker", f.Label),
			logutil.LF("marker_cluster", e.ClusterName),
			logutil.LF("current_cluster", clusterName))
		return false
	}
	return true
}

// Clear removes the marker; a missing file is success.
func (f File) Clear(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s marker: %w", f.Label, err)
	}
	return nil
}
