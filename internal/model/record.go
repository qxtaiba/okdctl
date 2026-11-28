package model

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/marker"
)

// ErrRecordMismatch reports a record whose envelope names a different
// cluster than its body.
var ErrRecordMismatch = errors.New("cluster record names a different cluster")

// ErrInfraID reports an infra-ID that is not <cluster>-<6 lowercase hex>.
var ErrInfraID = errors.New("infra-id is not <cluster>-<6 hex>")

// PreviewInfraID stands in for the record's infra-ID before up writes one;
// the MACs it yields are a preview and SaveRecord refuses it.
const PreviewInfraID = "preview"

var recordFile = marker.File{Label: "cluster record", Version: "1"}

// Record is the on-disk identity of a cluster, written by up before its first
// Proxmox write; Nodes names the day-1 nodes, masters then workers, so a
// worker added later is told apart from an original.
type Record struct {
	InfraID    string    `json:"infra_id"`
	Cluster    string    `json:"cluster"`
	Domain     string    `json:"domain"`
	Created    time.Time `json:"created"`
	Release    string    `json:"release"`
	InputsHash string    `json:"inputs_hash"`
	Nodes      []string  `json:"nodes"`
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

// LoadRecord reads the record at path; a missing file wraps os.ErrNotExist,
// another schema version wraps marker.ErrSchemaVersion and an envelope
// naming another cluster wraps ErrRecordMismatch.
func LoadRecord(path string) (*Record, error) {
	var body recordBody
	if err := recordFile.Read(path, &body); err != nil {
		return nil, err
	}
	switch {
	case body.ClusterName == "":
		return nil, fmt.Errorf("load cluster record %s: record has no cluster name", path)
	case body.ClusterName != body.Cluster:
		return nil, fmt.Errorf("load cluster record %s: %w (envelope %q, record %q)",
			path, ErrRecordMismatch, body.ClusterName, body.Cluster)
	}
	if err := body.checkInfraID(); err != nil {
		return nil, fmt.Errorf("load cluster record %s: %w", path, err)
	}
	return &body.Record, nil
}

func (r *Record) checkInfraID() error {
	rest, ok := strings.CutPrefix(r.InfraID, r.Cluster+"-")
	if r.Cluster == "" || !ok || len(rest) != 6 || strings.Trim(rest, "0123456789abcdef") != "" {
		return fmt.Errorf("infra_id %q: %w", r.InfraID, ErrInfraID)
	}
	return nil
}

// SaveRecord writes r atomically at mode 0600 with r.Cluster as the
// envelope's cluster name; an infra-ID NewInfraID would not produce is refused.
func SaveRecord(path string, r *Record) error {
	if err := r.checkInfraID(); err != nil {
		return fmt.Errorf("save cluster record: %w", err)
	}
	body := recordBody{Record: *r}
	if err := recordFile.Write(path, &body, r.Cluster); err != nil {
		return fmt.Errorf("save cluster record: %w", err)
	}
	return nil
}
