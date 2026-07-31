// Package deploy runs the okdctl deploy engine: phase orchestration with
// resume routing keyed on the on-disk deploy-state marker and the
// live-cluster setup guard.
package deploy

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/marker"
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
// this value (and update deployMarkerFile) only when the schema makes a
// breaking change.
const deployStateSchemaV2 = "v2"

// deployState records which deploy phase was active when the process last
// wrote the marker. Resume routing (resolveResumePhase) and destroy
// diagnostics (AnnounceState) read it back.
type deployState struct {
	marker.Envelope

	Phase deployPhase `json:"phase"`
}

var deployMarkerFile = marker.File{
	Label:   "deploy state",
	Version: deployStateSchemaV2,
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

// clearDeployMarker removes the marker on clean completion. A missing file is
// expected (write may have failed silently) and is not warned. When the
// remove itself fails, the marker is overwritten with a terminal completed
// state instead: leaving the stale phase in place would route the next
// deploy through a postinstall-only resume of a deploy that already
// finished.
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

// loadResumeMarker reads the deploy-state marker for the resume decision.
// Unreadable markers and markers failing the cluster-name guard (resume
// grants skip-wipe/skip-install power, so a marker must positively identify
// this cluster) are treated as absent.
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

// warnIfStaleResume flags resume markers old enough that resuming is likely
// to fail. The ignition certs openshift-install embeds for bootstrap are
// valid for 24 h, so an install-phase resume past that window will hang at
// the bootstrap wait unless bootstrap already completed; anything a week
// old is probably abandoned debris.
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

// InstallInProgress reports whether the deploy-state marker under workDir
// records an unfinished deploy for clusterName. Status phase derivation uses
// it to distinguish Installing from Pending/Stopped without trusting
// terraform-state presence.
func InstallInProgress(workDir, clusterName string) bool {
	m := loadResumeMarker(filepath.Join(workDir, StateFileName), clusterName)
	return m != nil && m.Phase != phaseCompleted
}

// AnnounceState emits a partial-deploy diagnostic on destroy entry.
// No-op when no marker exists or the marker fails the cluster-name guard.
// clusterName is cfg.Cluster.Name from the caller.
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
