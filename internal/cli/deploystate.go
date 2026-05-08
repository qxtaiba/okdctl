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
	Phase     string    `json:"phase"`
	RunID     string    `json:"run_id"`
	Timestamp time.Time `json:"timestamp"`
}

// markDeployPhase writes the marker for the given phase, warn-logging on
// failure (non-fatal — the marker is advisory).
func markDeployPhase(path, phase, runID string) {
	if err := writeDeployState(path, phase, runID); err != nil {
		tui.Warn("could not write deploy state marker", tui.LF("err", err))
	}
}

func writeDeployState(path, phase, runID string) error {
	data, err := json.Marshal(deployState{
		Phase:     phase,
		RunID:     runID,
		Timestamp: time.Now().UTC(),
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
// No-op when no marker exists.
func announceDeployState(path string) {
	ds, err := readDeployState(path)
	if err != nil {
		tui.Warn("could not read deploy state marker", tui.LF("err", err))
		return
	}
	if ds == nil {
		return
	}
	switch ds.Phase {
	case "prepare":
		tui.Warn("partial deploy detected — cancelled during prepare; terraform state is empty",
			tui.LF("run_id", ds.RunID))
		tui.Info("if VMs were not created, prefer 'okdctl cleanup' over destroy")
	case "install", "configure":
		tui.Warn("partial deploy detected — terraform state likely populated",
			tui.LF("phase", ds.Phase), tui.LF("run_id", ds.RunID))
		tui.Info("running destroy to remove provisioned resources")
	}
}
