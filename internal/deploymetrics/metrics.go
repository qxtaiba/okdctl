// Package deploymetrics provides a Prometheus text-format metrics recorder
// for the deploy run. No external dependency is required — the four metric
// families are in-memory aggregates serialised via fmt.Fprintf.
package deploymetrics

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/qxtaiba/okdctl/internal/distribution"
)

var histogramBuckets = []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300, 600}

// Recorder implements distribution.MetricsRecorder and serves the collected
// data as Prometheus text exposition via Handler.
type Recorder struct {
	mu          sync.Mutex
	stepTotal   map[string][2]int64
	stepDurSec  map[string][]float64
	currentStep string
	deployDur   float64
}

// NewRecorder returns an initialised Recorder ready for use.
func NewRecorder() *Recorder {
	return &Recorder{
		stepTotal:  make(map[string][2]int64),
		stepDurSec: make(map[string][]float64),
	}
}

// StepStarted marks id as the in-flight step for the current_step gauge.
func (r *Recorder) StepStarted(id distribution.StepID) {
	r.mu.Lock()
	r.currentStep = string(id)
	r.mu.Unlock()
}

// StepFinished records the completed step outcome and duration. Skipped
// steps are not counted in the histogram.
func (r *Recorder) StepFinished(result *distribution.StepResult) {
	if result.Skipped {
		return
	}
	id := string(result.StepID)
	secs := result.Duration.Seconds()
	r.mu.Lock()
	defer r.mu.Unlock()
	// lazy-init keeps the zero value of Recorder usable without NewRecorder.
	if r.stepTotal == nil {
		r.stepTotal = make(map[string][2]int64)
	}
	if r.stepDurSec == nil {
		r.stepDurSec = make(map[string][]float64)
	}
	c := r.stepTotal[id]
	if result.Success {
		c[0]++
	} else {
		c[1]++
	}
	r.stepTotal[id] = c
	r.stepDurSec[id] = append(r.stepDurSec[id], secs)
	if r.currentStep == id {
		r.currentStep = ""
	}
}

// DeployFinished records the overall deploy duration.
func (r *Recorder) DeployFinished(total time.Duration) {
	r.mu.Lock()
	r.deployDur = total.Seconds()
	r.mu.Unlock()
}

// Handler returns an http.Handler that renders all four metric families in
// Prometheus text format on every GET request.
func (r *Recorder) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		// Snapshot under lock, write outside: holding mu across net I/O
		// blocks every StepStarted/StepFinished call in the deploy path
		// on a slow scraper.
		r.mu.Lock()
		var b strings.Builder
		r.writeMetrics(&b)
		out := b.String()
		r.mu.Unlock()
		fmt.Fprint(w, out)
	})
}

func (r *Recorder) writeMetrics(b *strings.Builder) {
	fmt.Fprintln(b, "# HELP okdctl_deploy_step_total total step executions by outcome")
	fmt.Fprintln(b, "# TYPE okdctl_deploy_step_total counter")
	for id, c := range r.stepTotal {
		fmt.Fprintf(b, "okdctl_deploy_step_total{step=%q,outcome=\"success\"} %d\n", id, c[0])
		fmt.Fprintf(b, "okdctl_deploy_step_total{step=%q,outcome=\"failure\"} %d\n", id, c[1])
	}

	fmt.Fprintln(b, "# HELP okdctl_deploy_step_duration_seconds step execution duration in seconds")
	fmt.Fprintln(b, "# TYPE okdctl_deploy_step_duration_seconds histogram")
	for id, samples := range r.stepDurSec {
		counts := make([]int64, len(histogramBuckets))
		var sum float64
		for _, s := range samples {
			sum += s
			for i, bound := range histogramBuckets {
				if s <= bound {
					counts[i]++
				}
			}
		}
		var cumulative int64
		for i, bound := range histogramBuckets {
			cumulative += counts[i]
			fmt.Fprintf(b, "okdctl_deploy_step_duration_seconds_bucket{step=%q,le=\"%g\"} %d\n", id, bound, cumulative)
		}
		fmt.Fprintf(b, "okdctl_deploy_step_duration_seconds_bucket{step=%q,le=\"+Inf\"} %d\n", id, int64(len(samples)))
		fmt.Fprintf(b, "okdctl_deploy_step_duration_seconds_sum{step=%q} %g\n", id, sum)
		fmt.Fprintf(b, "okdctl_deploy_step_duration_seconds_count{step=%q} %d\n", id, int64(len(samples)))
	}

	fmt.Fprintln(b, "# HELP okdctl_deploy_current_step 1 when the labelled step is executing")
	fmt.Fprintln(b, "# TYPE okdctl_deploy_current_step gauge")
	if r.currentStep != "" {
		fmt.Fprintf(b, "okdctl_deploy_current_step{step=%q} 1\n", r.currentStep)
	}

	fmt.Fprintln(b, "# HELP okdctl_deploy_duration_seconds total deploy duration in seconds")
	fmt.Fprintln(b, "# TYPE okdctl_deploy_duration_seconds gauge")
	fmt.Fprintf(b, "okdctl_deploy_duration_seconds %g\n", r.deployDur)
}
