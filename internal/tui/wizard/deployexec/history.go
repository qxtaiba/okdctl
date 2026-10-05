package deployexec

import (
	"path/filepath"
	"time"

	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/marker"
)

// StepHistoryFileName is the per-step duration history file under
// <projectRoot>/okd-install, beside the deploy-state marker; the install
// instrument seeds its weight table and ETA from it.
const StepHistoryFileName = ".okdctl-step-history.json"

// stepHistorySchemaV1 is the current schema marker; change the payload in
// place and bump this on breaking shape changes — an unknown version reads
// as no history, never as an error.
const stepHistorySchemaV1 = "v1"

// historyEWMAAlpha is the weight one new run carries against the stored
// estimate: half, so a single outlier run bends the schedule without
// rewriting it.
const historyEWMAAlpha = 0.5

var stepHistoryFile = marker.File{
	Label:   "step history",
	Version: stepHistorySchemaV1,
}

// stepHistory is the on-disk payload: per-step EWMA durations in seconds,
// keyed by the engine's step ids.
type stepHistory struct {
	marker.Envelope

	Steps map[string]float64 `json:"steps"`
}

// LoadStepHistory reads the per-step duration history under workDir for
// clusterName. Absent, corrupt, unknown-version, or foreign-cluster files
// all read as nil — a prediction seed must never block or skew a deploy it
// does not belong to.
func LoadStepHistory(workDir, clusterName string) map[distribution.StepID]time.Duration {
	var h stepHistory
	found, err := stepHistoryFile.Read(filepath.Join(workDir, StepHistoryFileName), &h)
	if err != nil || !found || !stepHistoryFile.Trusted(&h, clusterName) {
		return nil
	}
	out := make(map[distribution.StepID]time.Duration, len(h.Steps))
	for id, secs := range h.Steps {
		if secs > 0 {
			out[distribution.StepID(id)] = time.Duration(secs * float64(time.Second))
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// RecordStepHistory folds one run's measured step durations into the
// history under workDir: a step already known moves by EWMA
// (historyEWMAAlpha), a new one seeds with its raw duration, and skipped,
// failed, or unmeasured steps contribute nothing. Existing entries the run
// never reached are kept — a resumed run must not erase the schedule of the
// phases it skipped.
func RecordStepHistory(workDir, runID, clusterName string, results []distribution.StepResult) error {
	steps := map[string]float64{}
	if prev := LoadStepHistory(workDir, clusterName); prev != nil {
		for id, d := range prev {
			steps[string(id)] = d.Seconds()
		}
	}
	for i := range results {
		r := &results[i]
		if !r.Success || r.Skipped || r.Duration <= 0 {
			continue
		}
		secs := r.Duration.Seconds()
		if prev, ok := steps[string(r.StepID)]; ok {
			secs = historyEWMAAlpha*secs + (1-historyEWMAAlpha)*prev
		}
		steps[string(r.StepID)] = secs
	}
	return stepHistoryFile.Write(filepath.Join(workDir, StepHistoryFileName),
		&stepHistory{Steps: steps}, runID, clusterName)
}
