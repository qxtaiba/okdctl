package deployexec

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/setup"
)

func TestStepHistoryRoundTripAndEWMAMerge(t *testing.T) {
	dir := t.TempDir()

	first := []distribution.StepResult{
		{StepID: setup.StepInstallPackages, Success: true, Duration: 40 * time.Second},
		{StepID: setup.StepDownloadTools, Success: true, Duration: 100 * time.Second},
	}
	if err := RecordStepHistory(dir, "run-1", "homelab", first); err != nil {
		t.Fatalf("RecordStepHistory: %v", err)
	}

	got := LoadStepHistory(dir, "homelab")
	if got[setup.StepInstallPackages] != 40*time.Second {
		t.Errorf("first run seeds the raw duration, got %v", got[setup.StepInstallPackages])
	}

	second := []distribution.StepResult{
		{StepID: setup.StepInstallPackages, Success: true, Duration: 20 * time.Second},
	}
	if err := RecordStepHistory(dir, "run-2", "homelab", second); err != nil {
		t.Fatalf("RecordStepHistory: %v", err)
	}
	got = LoadStepHistory(dir, "homelab")
	if got[setup.StepInstallPackages] != 30*time.Second {
		t.Errorf("EWMA(0.5) of 40s and 20s = 30s, got %v", got[setup.StepInstallPackages])
	}
	if got[setup.StepDownloadTools] != 100*time.Second {
		t.Errorf("a step absent from the new run keeps its history, got %v", got[setup.StepDownloadTools])
	}
}

func TestStepHistorySkipsUnusableResults(t *testing.T) {
	dir := t.TempDir()
	results := []distribution.StepResult{
		{StepID: setup.StepInstallPackages, Success: true, Skipped: true, Duration: 40 * time.Second},
		{StepID: setup.StepDownloadTools, Success: false, Duration: 40 * time.Second},
		{StepID: setup.StepEnsureWorkDir, Success: true, Duration: 0},
	}
	if err := RecordStepHistory(dir, "run-1", "homelab", results); err != nil {
		t.Fatalf("RecordStepHistory: %v", err)
	}
	if got := LoadStepHistory(dir, "homelab"); len(got) != 0 {
		t.Errorf("skipped, failed, and zero-duration steps must not seed predictions, got %v", got)
	}
}

func TestStepHistoryAbsentOrForeignReadsAsNil(t *testing.T) {
	dir := t.TempDir()
	if got := LoadStepHistory(dir, "homelab"); got != nil {
		t.Errorf("no history file must read as nil, got %v", got)
	}

	results := []distribution.StepResult{
		{StepID: setup.StepInstallPackages, Success: true, Duration: 40 * time.Second},
	}
	if err := RecordStepHistory(dir, "run-1", "prod", results); err != nil {
		t.Fatalf("RecordStepHistory: %v", err)
	}
	if got := LoadStepHistory(dir, "homelab"); got != nil {
		t.Errorf("another cluster's history must not seed this one's schedule, got %v", got)
	}

	// A corrupt file reads as absent too — a prediction seed must never
	// block a deploy.
	path := filepath.Join(dir, StepHistoryFileName)
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := LoadStepHistory(dir, "prod"); got != nil {
		t.Errorf("a corrupt history file must read as nil, got %v", got)
	}
}
