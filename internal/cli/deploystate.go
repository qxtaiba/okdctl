package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/qxtaiba/okdctl/internal/system"
)

// deployState records which deploy phase was active when the process last
// wrote the marker. runDestroy reads it back to emit a phase-specific hint.
type deployState struct {
	Phase     string    `json:"phase"`
	RunID     string    `json:"run_id"`
	Timestamp time.Time `json:"timestamp"`
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
