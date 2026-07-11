package deploymetrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/distribution"
)

// TestRecorder_ZeroValueStepFinished verifies a zero-value Recorder does not
// panic when StepFinished writes its maps before any explicit initialisation.
func TestRecorder_ZeroValueStepFinished(t *testing.T) {
	var r Recorder
	result := &distribution.StepResult{
		StepID:   distribution.StepID("test-step"),
		Success:  true,
		Duration: time.Second,
	}
	r.StepFinished(result)
	if r.stepTotal["test-step"][0] != 1 {
		t.Fatalf("expected success count 1, got %d", r.stepTotal["test-step"][0])
	}
}

// TestRecorder_HistogramBucketsCumulative feeds known duration samples and
// verifies the exposed bucket counts are monotonically non-decreasing and
// that no le<+Inf bucket exceeds the +Inf bucket, matching Prometheus
// histogram semantics.
func TestRecorder_HistogramBucketsCumulative(t *testing.T) {
	tests := []struct {
		name    string
		samples []float64
	}{
		{name: "single sample below smallest bucket", samples: []float64{0.05}},
		{name: "samples spread across buckets", samples: []float64{0.05, 0.05, 0.2, 1.5, 50, 400}},
		{name: "samples above largest bucket", samples: []float64{700, 900}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRecorder()
			for _, s := range tt.samples {
				r.StepFinished(&distribution.StepResult{
					StepID:   distribution.StepID("test-step"),
					Success:  true,
					Duration: time.Duration(s * float64(time.Second)),
				})
			}

			counts, infCount := parseBucketCounts(t, scrapeMetrics(r))

			var prev int64
			for i, c := range counts {
				if c < prev {
					t.Fatalf("bucket %d count %d is less than previous bucket count %d", i, c, prev)
				}
				if c > infCount {
					t.Fatalf("bucket %d count %d exceeds +Inf bucket count %d", i, c, infCount)
				}
				prev = c
			}
			if infCount != int64(len(tt.samples)) {
				t.Fatalf("+Inf bucket count = %d, want %d", infCount, len(tt.samples))
			}
		})
	}
}

// TestRecorder_HandlerServesAllMetricFamilies drives a full step lifecycle
// through Handler and checks each metric family renders, a skipped step is
// excluded, and the current-step gauge tracks the most recent StepStarted
// call.
func TestRecorder_HandlerServesAllMetricFamilies(t *testing.T) {
	r := NewRecorder()
	r.StepStarted(distribution.StepID("apply"))
	r.StepFinished(&distribution.StepResult{
		StepID:   distribution.StepID("apply"),
		Success:  true,
		Duration: 250 * time.Millisecond,
	})
	r.StepFinished(&distribution.StepResult{
		StepID:   distribution.StepID("apply"),
		Success:  false,
		Duration: 500 * time.Millisecond,
	})
	r.StepFinished(&distribution.StepResult{
		StepID:  distribution.StepID("skip-me"),
		Skipped: true,
	})
	r.StepStarted(distribution.StepID("verify"))
	r.DeployFinished(90 * time.Second)

	body := scrapeMetrics(r)
	for _, want := range []string{
		`okdctl_deploy_step_total{step="apply",outcome="success"} 1`,
		`okdctl_deploy_step_total{step="apply",outcome="failure"} 1`,
		`okdctl_deploy_current_step{step="verify"} 1`,
		`okdctl_deploy_duration_seconds 90`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("output missing %q; full output:\n%s", want, body)
		}
	}
	if strings.Contains(body, `step="skip-me"`) {
		t.Fatalf("skipped step should not appear in output:\n%s", body)
	}
}

func scrapeMetrics(r *Recorder) string {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/metrics", http.NoBody)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)
	return rec.Body.String()
}

func parseBucketCounts(t *testing.T, body string) (counts []int64, infCount int64) {
	t.Helper()
	bucketRe := regexp.MustCompile(`okdctl_deploy_step_duration_seconds_bucket\{step="test-step",le="([^"]+)"\} (\d+)`)
	for _, m := range bucketRe.FindAllStringSubmatch(body, -1) {
		count, err := strconv.ParseInt(m[2], 10, 64)
		if err != nil {
			t.Fatalf("parsing bucket count %q: %v", m[2], err)
		}
		if m[1] == "+Inf" {
			infCount = count
			continue
		}
		counts = append(counts, count)
	}
	return counts, infCount
}
