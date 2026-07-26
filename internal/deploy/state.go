// Package deploy runs the okdctl deploy engine: phase orchestration with
// resume routing keyed on the on-disk deploy-state marker and the
// live-cluster setup guard.
package deploy

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
)

// deployPhase identifies which phase of the deploy sequence was active when
// the state marker was last written.
type deployPhase string

const (
	phaseSetup       deployPhase = "setup"
	phaseInstall     deployPhase = "install"
	phasePostInstall deployPhase = "postinstall"

	// phaseCompleted is the terminal marker state clearDeployMarker falls
	// back to when it cannot remove the file: resume routing and destroy
	// diagnostics treat it as no marker at all, so a stale-but-unremovable
	// marker can never route the next deploy through a postinstall resume.
	phaseCompleted deployPhase = "completed"
)

// deployStateSchemaV2 is the current deploy-state JSON schema marker. Bump
// this value (and update readDeployState) only when the schema makes a
// breaking change. v1 markers used prepare/configure phase names;
// readDeployState maps them onto the v2 vocabulary so an interrupted deploy
// from an older binary still resumes correctly.
const (
	deployStateSchemaV1 = "v1"
	deployStateSchemaV2 = "v2"
)

// deployState records which deploy phase was active when the process last
// wrote the marker. Resume routing (resolveResumePhase) and destroy
// diagnostics (announceDeployState) read it back.
type deployState struct {
	SchemaVersion string      `json:"schema_version"`
	Phase         deployPhase `json:"phase"`
	RunID         string      `json:"run_id"`
	Timestamp     time.Time   `json:"timestamp"`
	ClusterName   string      `json:"cluster_name,omitempty"`
}

// markDeployPhaseFatal writes the marker for the given phase and returns any
// write error. The marker is load-bearing routing state — resolveResumePhase
// keys the pre-setup wipe on its Phase — so a failed write must abort the
// deploy: proceeding would leave a stale setup-phase marker that routes a
// post-install resume through the wipe.
func markDeployPhaseFatal(path string, phase deployPhase, runID, clusterName string) error {
	if err := writeDeployState(path, phase, runID, clusterName); err != nil {
		return fmt.Errorf("write deploy state marker: %w", err)
	}
	return nil
}

// clearDeployMarker removes the marker on clean completion. ErrNotExist is
// expected (write may have failed silently) and is not warned. When the
// remove itself fails, the marker is overwritten with a terminal completed
// state instead: leaving the stale phase in place would route the next
// deploy through a postinstall-only resume of a deploy that already
// finished.
func clearDeployMarker(path, runID, clusterName string) {
	err := os.Remove(path)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return
	}
	if writeErr := writeDeployState(path, phaseCompleted, runID, clusterName); writeErr != nil {
		tui.Warn("could not remove deploy state marker",
			tui.LF("remove_err", err), tui.LF("mark_completed_err", writeErr))
	}
}

func writeDeployState(path string, phase deployPhase, runID, clusterName string) error {
	data, err := json.Marshal(deployState{
		SchemaVersion: deployStateSchemaV2,
		Phase:         phase,
		RunID:         runID,
		Timestamp:     time.Now().UTC(),
		ClusterName:   clusterName,
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
	switch s.SchemaVersion {
	case deployStateSchemaV2:
	case deployStateSchemaV1:
		s.Phase = migrateV1Phase(s.Phase)
	default:
		tui.Warn("ignoring deploy-state with unknown schema_version",
			tui.LF("schema_version", s.SchemaVersion), tui.LF("expected", deployStateSchemaV2))
		return nil, nil
	}
	return &s, nil
}

// migrateV1Phase maps the retired v1 phase vocabulary onto the package names
// v2 records. Values it does not recognize pass through unchanged so
// resolveResumePhase's unknown-phase-treated-as-absent handling still applies.
func migrateV1Phase(p deployPhase) deployPhase {
	switch p {
	case "prepare":
		return phaseSetup
	case "configure":
		return phasePostInstall
	}
	return p
}

// loadResumeMarker reads the deploy-state marker for the resume decision.
// Unreadable markers, markers naming a different cluster, and markers with
// no cluster name at all (older binaries omitted it) are treated as absent:
// resume grants skip-wipe/skip-install power, so a marker must positively
// identify this cluster before it is trusted.
func loadResumeMarker(path, clusterName string) *deployState {
	marker, err := readDeployState(path)
	if err != nil {
		tui.Warn("could not read deploy state marker; treating as absent", tui.LF("err", err))
		return nil
	}
	if marker == nil {
		return nil
	}
	if marker.ClusterName == "" {
		tui.Warn("deploy state marker has no cluster name; treating as absent",
			tui.LF("current_cluster", clusterName))
		return nil
	}
	if marker.ClusterName != clusterName {
		tui.Warn("deploy state marker is from a different cluster, ignoring",
			tui.LF("marker_cluster", marker.ClusterName), tui.LF("current_cluster", clusterName))
		return nil
	}
	return marker
}

// resolveResumePhase decides where an interrupted deploy resumes. An install
// or postinstall marker means live VMs booted with the ignition/CA in the
// work directory and cluster-config/auth holds the only copy of the cluster's
// auth bundle — never wipe or regenerate identity material the marker says a
// live cluster depends on, so those resumes route past setup (and its
// pre-setup wipe) entirely. A setup marker guarantees no VM has booted
// with the current work directory's contents, so resuming through the wipe
// is safe. FreshDeploy restarts from setup: the operator accepted
// credential loss by passing --fresh. A marker with an unrecognized phase is
// treated as absent — it must not vouch for a guard bypass.
func resolveResumePhase(markerPath, clusterName string, freshDeploy bool) (deployPhase, *deployState) {
	marker := loadResumeMarker(markerPath, clusterName)
	if freshDeploy || marker == nil {
		return phaseSetup, marker
	}
	switch marker.Phase {
	case phaseInstall, phasePostInstall:
		warnIfStaleResume(marker)
		return marker.Phase, marker
	case phaseSetup:
		return phaseSetup, marker
	case phaseCompleted:
		return phaseSetup, nil
	}
	tui.Warn("deploy state marker has unknown phase; treating as absent",
		tui.LF("phase", string(marker.Phase)))
	return phaseSetup, nil
}

// warnIfStaleResume flags resume markers old enough that resuming is likely
// to fail. The ignition certs openshift-install embeds for bootstrap are
// valid for 24 h, so an install-phase resume past that window will hang at
// the bootstrap wait unless bootstrap already completed; anything a week
// old is probably abandoned debris.
func warnIfStaleResume(marker *deployState) {
	age := time.Since(marker.Timestamp)
	switch {
	case marker.Phase == phaseInstall && age >= 24*time.Hour:
		tui.Warn("deploy state marker is older than the 24h bootstrap ignition cert validity; resume may fail at bootstrap wait",
			tui.LF("marker_age", age.Round(time.Hour).String()))
		tui.Info("if bootstrap never completed, run 'okdctl destroy' then re-deploy with --fresh")
	case age >= 7*24*time.Hour:
		tui.Warn("deploy state marker is likely stale",
			tui.LF("marker_age", age.Round(time.Hour).String()))
		tui.Info("re-run with --fresh to restart from scratch instead (credentials will be lost)")
	}
}

// AnnounceState emits a partial-deploy diagnostic on destroy entry.
// No-op when no marker exists. clusterName is cfg.Cluster.Name from the
// caller; a non-empty ClusterName mismatch means the marker belongs to a
// different cluster and is ignored.
func AnnounceState(path, clusterName string) {
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
		if modAge := time.Since(info.ModTime()); modAge >= 7*24*time.Hour {
			extra = append(extra,
				tui.LF("marker_age", modAge.Round(time.Hour).String()),
				tui.LF("stale", true))
		}
	}
	switch ds.Phase {
	case phaseCompleted:
		return
	case phaseSetup:
		tui.Warn("partial deploy detected — cancelled during setup; terraform state is empty",
			append([]tui.LogField{tui.LF("run_id", ds.RunID)}, extra...)...)
		tui.Info("if VMs were not created, prefer 'okdctl cleanup' over destroy")
	case phaseInstall, phasePostInstall:
		tui.Warn("partial deploy detected — terraform state likely populated",
			append([]tui.LogField{tui.LF("phase", ds.Phase), tui.LF("run_id", ds.RunID)}, extra...)...)
		tui.Info("running destroy to remove provisioned resources")
	}
}
