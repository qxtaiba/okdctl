package model

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/marker"
)

// ErrRecordMismatch reports a record whose envelope names a different
// cluster than its body.
var ErrRecordMismatch = errors.New("cluster record names a different cluster")

var recordFile = marker.File{Label: "cluster record", Version: "1"}

// Record is the on-disk identity of a cluster, written once at define time.
type Record struct {
	InfraID    string    `json:"infra_id"`
	Cluster    string    `json:"cluster"`
	Domain     string    `json:"domain"`
	Created    time.Time `json:"created"`
	Release    string    `json:"release"`
	InputsHash string    `json:"inputs_hash"`
}

type recordBody struct {
	marker.Envelope
	Record
}

// NewInfraID returns "<cluster>-<6 lowercase hex>" drawn from crypto/rand.
func NewInfraID(cluster string) string {
	var b [3]byte
	rand.Read(b[:])
	return cluster + "-" + hex.EncodeToString(b[:])
}

// LoadRecord reads the record at path; a missing file wraps os.ErrNotExist
// and an envelope naming another cluster wraps ErrRecordMismatch.
func LoadRecord(path string) (*Record, error) {
	var body recordBody
	found, err := recordFile.Read(path, &body)
	if err != nil {
		return nil, err
	}
	if !found {
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("load cluster record: %w", err)
		}
		return nil, fmt.Errorf("load cluster record %s: unsupported schema_version %q", path, body.SchemaVersion)
	}
	switch {
	case body.ClusterName == "":
		return nil, fmt.Errorf("load cluster record %s: record has no cluster name", path)
	case body.ClusterName != body.Cluster:
		return nil, fmt.Errorf("load cluster record %s: %w (envelope %q, record %q)",
			path, ErrRecordMismatch, body.ClusterName, body.Cluster)
	}
	return &body.Record, nil
}

// SaveRecord writes r atomically at mode 0600 with r.Cluster as the
// envelope's cluster name.
func SaveRecord(path string, r *Record) error {
	body := recordBody{Record: *r}
	if err := recordFile.Write(path, &body, logutil.RunID(), r.Cluster); err != nil {
		return fmt.Errorf("save cluster record: %w", err)
	}
	return nil
}
