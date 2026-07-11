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

// deployPhase identifies which phase of the deploy sequence was active when
// the state marker was last written.
type deployPhase string

const (
	phasePrepare   deployPhase = "prepare"
	phaseInstall   deployPhase = "install"
	phaseConfigure deployPhase = "configure"
)

// deployStateSchemaV1 is the current deploy-state JSON schema marker. Bump
// this value (and update readDeployState) only when the schema makes a
// breaking change.
const deployStateSchemaV1 = "v1"

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
// deploy: proceeding would leave a stale prepare-phase marker that routes a
// post-install resume through the wipe.
func markDeployPhaseFatal(path string, phase deployPhase, runID, clusterName string) error {
	if err := writeDeployState(path, phase, runID, clusterName); err != nil {
		return fmt.Errorf("write deploy state marker: %w", err)
	}
	return nil
}

// clearDeployMarker removes the marker on clean completion. ErrNotExist is
// expected (write may have failed silently) and is not warned.
func clearDeployMarker(path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		tui.Warn("could not remove deploy state marker", tui.LF("err", err))
	}
}

func writeDeployState(path string, phase deployPhase, runID, clusterName string) error {
	data, err := json.Marshal(deployState{
		SchemaVersion: deployStateSchemaV1,
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
	if s.SchemaVersion != deployStateSchemaV1 {
		tui.Warn("ignoring deploy-state with unknown schema_version",
			tui.LF("schema_version", s.SchemaVersion), tui.LF("expected", deployStateSchemaV1))
		return nil, nil
	}
	return &s, nil
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
// or configure marker means live VMs booted with the ignition/CA in the work
// directory and cluster-config/auth holds the only copy of the cluster's
// auth bundle — never wipe or regenerate identity material the marker says a
// live cluster depends on, so those resumes route past prepare (and its
// pre-setup wipe) entirely. A prepare marker guarantees no VM has booted
// with the current work directory's contents, so resuming through the wipe
// is safe. FreshDeploy restarts from prepare: the operator accepted
// credential loss by passing --fresh. A marker with an unrecognized phase is
// treated as absent — it must not vouch for a guard bypass.
func resolveResumePhase(markerPath, clusterName string, freshDeploy bool) (deployPhase, *deployState) {
	marker := loadResumeMarker(markerPath, clusterName)
	if freshDeploy || marker == nil {
		return phasePrepare, marker
	}
	switch marker.Phase {
	case phaseInstall, phaseConfigure:
		warnIfStaleResume(marker)
		return marker.Phase, marker
	case phasePrepare:
		return phasePrepare, marker
	}
	tui.Warn("deploy state marker has unknown phase; treating as absent",
		tui.LF("phase", string(marker.Phase)))
	return phasePrepare, nil
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
			tui.LF("marker_age", fmt.Sprintf("%d days", int(age.Hours()/24))))
		tui.Info("re-run with --fresh to restart from scratch instead (credentials will be lost)")
	}
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
	case phasePrepare:
		tui.Warn("partial deploy detected — cancelled during prepare; terraform state is empty",
			append([]tui.LogField{tui.LF("run_id", ds.RunID)}, extra...)...)
		tui.Info("if VMs were not created, prefer 'okdctl cleanup' over destroy")
	case phaseInstall, phaseConfigure:
		tui.Warn("partial deploy detected — terraform state likely populated",
			append([]tui.LogField{tui.LF("phase", ds.Phase), tui.LF("run_id", ds.RunID)}, extra...)...)
		tui.Info("running destroy to remove provisioned resources")
	}
}
