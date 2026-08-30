// Package deploy runs the okdctl deploy engine: phase orchestration with
// resume routing keyed on the on-disk deploy-state marker and the live-cluster setup guard.
package deploy

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/marker"
)

// deployPhase identifies which phase was active when the state marker was last written.
type deployPhase string

const (
	phaseSetup       deployPhase = "setup"
	phaseInstall     deployPhase = "install"
	phasePostInstall deployPhase = "postinstall"

	// phaseCompleted is clearDeployMarker's fallback when it can't remove the
	// file; resume routing and destroy diagnostics treat it as no marker, so
	// a stale marker can't route a postinstall resume.
	phaseCompleted deployPhase = "completed"
)

// deployStateSchemaV2 is the current schema marker; bump it (and
// deployMarkerFile) only on breaking schema changes.
const deployStateSchemaV2 = "v2"

// deployState records the phase active when the marker was last written;
// resolveResumePhase and AnnounceState read it back.
type deployState struct {
	marker.Envelope

	Phase deployPhase `json:"phase"`
}

var deployMarkerFile = marker.File{
	Label:   "deploy state",
	Version: deployStateSchemaV2,
}

// markDeployPhaseFatal writes the phase marker; a failed write must abort
// the deploy — resolveResumePhase keys the pre-setup wipe on Phase, so a
// stale marker could route a post-install resume through the wipe.
func markDeployPhaseFatal(path string, phase deployPhase, runID, clusterName string) error {
	if err := writeDeployState(path, phase, runID, clusterName); err != nil {
		return fmt.Errorf("write deploy state marker: %w", err)
	}
	return nil
}

// clearDeployMarker removes the marker on completion; a missing file isn't
// warned. If removal fails, it overwrites the marker with phaseCompleted
// instead — leaving the stale phase would route a resume of a deploy that
// already finished.
func clearDeployMarker(path, runID, clusterName string) {
	err := deployMarkerFile.Clear(path)
	if err == nil {
		return
	}
	if writeErr := writeDeployState(path, phaseCompleted, runID, clusterName); writeErr != nil {
		logutil.Warn("could not remove deploy state marker",
			logutil.LF("remove_err", err), logutil.LF("mark_completed_err", writeErr))
	}
}

func writeDeployState(path string, phase deployPhase, runID, clusterName string) error {
	return deployMarkerFile.Write(path, &deployState{Phase: phase}, runID, clusterName)
}

func readDeployState(path string) (*deployState, error) {
	var s deployState
	found, err := deployMarkerFile.Read(path, &s)
	if err != nil || !found {
		return nil, err
	}
	return &s, nil
}

// loadResumeMarker treats unreadable markers, or ones failing the
// cluster-name guard, as absent — resume grants skip-wipe/skip-install
// power, so a marker must positively identify this cluster.
func loadResumeMarker(path, clusterName string) *deployState {
	m, err := readDeployState(path)
	if err != nil {
		logutil.Warn("could not read deploy state marker; treating as absent", logutil.LF("err", err))
		return nil
	}
	if m == nil || !deployMarkerFile.Trusted(m, clusterName) {
		return nil
	}
	return m
}

// resolveResumePhase decides where an interrupted deploy resumes. Install and
// postinstall markers mean live VMs depend on identity material the work
// directory holds, so those resumes must skip setup's pre-setup wipe
// entirely; a setup marker guarantees no VM has booted yet, so the wipe is
// safe. FreshDeploy always restarts from setup (the operator accepted
// credential loss via --fresh); an unrecognized phase is treated as absent
// so it can't vouch for a guard bypass.
func resolveResumePhase(markerPath, clusterName string, freshDeploy bool) (deployPhase, *deployState) {
	m := loadResumeMarker(markerPath, clusterName)
	if freshDeploy || m == nil {
		return phaseSetup, m
	}
	switch m.Phase {
	case phaseInstall, phasePostInstall:
		warnIfStaleResume(m)
		return m.Phase, m
	case phaseSetup:
		return phaseSetup, m
	case phaseCompleted:
		return phaseSetup, nil
	}
	logutil.Warn("deploy state marker has unknown phase; treating as absent",
		logutil.LF("phase", string(m.Phase)))
	return phaseSetup, nil
}

// warnIfStaleResume flags markers unlikely to resume cleanly: install-phase
// markers older than the bootstrap ignition certs' 24h validity may hang at
// the bootstrap wait; anything a week old is probably abandoned debris.
func warnIfStaleResume(m *deployState) {
	switch {
	case m.Phase == phaseInstall && m.Age() >= 24*time.Hour:
		logutil.Warn("deploy state marker is older than the 24h bootstrap ignition cert validity; resume may fail at bootstrap wait",
			logutil.LF("marker_age", m.Age().Round(time.Hour).String()))
		logutil.Info("if bootstrap never completed, run 'okdctl destroy' then re-deploy with --fresh")
	case m.Stale():
		logutil.Warn("deploy state marker is likely stale",
			logutil.LF("marker_age", m.Age().Round(time.Hour).String()))
		logutil.Info("re-run with --fresh to restart from scratch instead (credentials will be lost)")
	}
}

// InstallInProgress reports whether the marker under workDir records an
// unfinished deploy for clusterName.
func InstallInProgress(workDir, clusterName string) bool {
	m := loadResumeMarker(filepath.Join(workDir, StateFileName), clusterName)
	return m != nil && m.Phase != phaseCompleted
}

// AnnounceState emits a partial-deploy diagnostic on destroy entry; no-op
// when no marker exists or it fails the cluster-name guard.
func AnnounceState(path, clusterName string) {
	ds, err := readDeployState(path)
	if err != nil {
		logutil.Warn("could not read deploy state marker", logutil.LF("err", err))
		return
	}
	if ds == nil || !deployMarkerFile.Trusted(ds, clusterName) {
		return
	}
	var extra []logutil.LogField
	if ds.Stale() {
		extra = append(extra,
			logutil.LF("marker_age", ds.Age().Round(time.Hour).String()),
			logutil.LF("stale", true))
	}
	switch ds.Phase {
	case phaseCompleted:
		return
	case phaseSetup:
		logutil.Warn("partial deploy detected — cancelled during setup; terraform state is empty",
			append([]logutil.LogField{logutil.LF("run_id", ds.RunID)}, extra...)...)
		logutil.Info("if VMs were not created, prefer 'okdctl cleanup' over destroy")
	case phaseInstall, phasePostInstall:
		logutil.Warn("partial deploy detected — terraform state likely populated",
			append([]logutil.LogField{logutil.LF("phase", ds.Phase), logutil.LF("run_id", ds.RunID)}, extra...)...)
		logutil.Info("running destroy to remove provisioned resources")
	}
}
