package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
)

// deployState records which deploy phase was active when the process last
// wrote the marker. runDestroy reads it back to emit a phase-specific hint.
type deployState struct {
	Phase       string    `json:"phase"`
	RunID       string    `json:"run_id"`
	Timestamp   time.Time `json:"timestamp"`
	ClusterName string    `json:"cluster_name,omitempty"`
}

// markDeployPhaseFatal writes the marker for the prepare phase and returns
// any write error. The first marker write is fatal so a write failure cannot
// produce a silently-accumulated stale marker.
func markDeployPhaseFatal(path, phase, runID, clusterName string) error {
	if err := writeDeployState(path, phase, runID, clusterName); err != nil {
		return fmt.Errorf("write deploy state marker: %w", err)
	}
	return nil
}

// markDeployPhase writes the marker for the given phase, warn-logging on
// failure (non-fatal — the marker is advisory for subsequent phases).
func markDeployPhase(path, phase, runID, clusterName string) {
	if err := writeDeployState(path, phase, runID, clusterName); err != nil {
		tui.Warn("could not write deploy state marker", tui.LF("err", err))
	}
}

// clearDeployMarker removes the marker on clean completion. ErrNotExist is
// expected (write may have failed silently) and is not warned.
func clearDeployMarker(path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		tui.Warn("could not remove deploy state marker", tui.LF("err", err))
	}
}

func writeDeployState(path, phase, runID, clusterName string) error {
	data, err := json.Marshal(deployState{
		Phase:       phase,
		RunID:       runID,
		Timestamp:   time.Now().UTC(),
		ClusterName: clusterName,
	})
	if err != nil {
		return fmt.Errorf("marshal deploy state: %w", err)
	}
	return system.AtomicWrite(path, data, 0o600)
}

func readDeployState(path string) (*deployState, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read deploy state: %w", err)
	}
	var s deployState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse deploy state: %w", err)
	}
	return &s, nil
}

// announceDeployState emits a partial-deploy diagnostic on destroy entry.
// No-op when no marker exists. clusterName is cfg.Cluster.Name from the
// caller; a non-empty ClusterName mismatch means the marker belongs to a
// different cluster and is ignored.
func announceDeployState(path, clusterName string) {
	info, statErr := os.Stat(path)
	ds, err := readDeployState(path)
	if err != nil {
		tui.Warn("could not read deploy state marker", tui.LF("err", err))
		return
	}
	if ds == nil {
		return
	}
	if ds.ClusterName != "" && ds.ClusterName != clusterName {
		tui.Warn("deploy state marker is from a different cluster, ignoring",
			tui.LF("marker_cluster", ds.ClusterName), tui.LF("current_cluster", clusterName))
		return
	}
	var extra []tui.LogField
	if statErr == nil {
		days := int(time.Since(info.ModTime()).Hours() / 24)
		if days >= 7 {
			extra = append(extra, tui.LF("marker_age", fmt.Sprintf("%d days — likely stale", days)))
		}
	}
	switch ds.Phase {
	case "prepare":
		tui.Warn("partial deploy detected — cancelled during prepare; terraform state is empty",
			append([]tui.LogField{tui.LF("run_id", ds.RunID)}, extra...)...)
		tui.Info("if VMs were not created, prefer 'okdctl cleanup' over destroy")
	case "install", "configure":
		tui.Warn("partial deploy detected — terraform state likely populated",
			append([]tui.LogField{tui.LF("phase", ds.Phase), tui.LF("run_id", ds.RunID)}, extra...)...)
		tui.Info("running destroy to remove provisioned resources")
	}
}
