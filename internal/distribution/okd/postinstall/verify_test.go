package postinstall

import (
	"strings"
	"testing"
)

func TestParseOperatorDegradation(t *testing.T) {
	cases := []struct {
		name         string
		payload      string
		wantDegraded []string
		wantErr      string
	}{
		{
			name: "no degraded operators",
			payload: `{
	"items": [
		{"metadata":{"name":"authentication"},"status":{"conditions":[{"type":"Degraded","status":"False"},{"type":"Available","status":"True"}]}},
		{"metadata":{"name":"dns"},"status":{"conditions":[{"type":"Degraded","status":"False"},{"type":"Available","status":"True"}]}}
	]
}`,
			wantDegraded: nil,
		},
		{
			name: "one degraded operator",
			payload: `{
	"items": [
		{"metadata":{"name":"authentication"},"status":{"conditions":[{"type":"Degraded","status":"True"}]}},
		{"metadata":{"name":"dns"},"status":{"conditions":[{"type":"Degraded","status":"False"}]}}
	]
}`,
			wantDegraded: []string{"authentication"},
		},
		{
			name: "operator with no degraded condition is not counted",
			payload: `{
	"items": [
		{"metadata":{"name":"authentication"},"status":{"conditions":[{"type":"Available","status":"True"}]}}
	]
}`,
			wantDegraded: nil,
		},
		{
			name:    "malformed json returns parse error",
			payload: `not-json`,
			wantErr: "parse",
		},
		{
			name:         "empty items list returns no degraded",
			payload:      `{"items":[]}`,
			wantDegraded: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseOperatorDegradation([]byte(tc.payload))
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q; got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %q; want substring %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.wantDegraded) {
				t.Fatalf("degraded = %v; want %v", got, tc.wantDegraded)
			}
			for i, name := range tc.wantDegraded {
				if got[i] != name {
					t.Errorf("degraded[%d] = %q; want %q", i, got[i], name)
				}
			}
		})
	}
}

func TestParseNodeReadiness(t *testing.T) {
	cases := []struct {
		name      string
		payload   string
		wantReady int
		wantTotal int
		wantErr   string
	}{
		{
			// Regression: the old strings.Contains parser misclassified these as not-ready.
			name: "scheduling-disabled node with Ready=True counts as ready",
			payload: `{
	"items": [
		{"metadata":{"name":"node-0"},"status":{"conditions":[{"type":"Ready","status":"True"},{"type":"NetworkUnavailable","status":"False"}]}},
		{"metadata":{"name":"node-1"},"status":{"conditions":[{"type":"MemoryPressure","status":"False"},{"type":"Ready","status":"True"}]}}
	]
}`,
			wantReady: 2,
			wantTotal: 2,
		},
		{
			name: "one node not ready",
			payload: `{
	"items": [
		{"metadata":{"name":"node-0"},"status":{"conditions":[{"type":"Ready","status":"False"}]}}
	]
}`,
			wantReady: 0,
			wantTotal: 1,
		},
		{
			name:    "malformed json returns parse error",
			payload: `not-json`,
			wantErr: "parse",
		},
		{
			name:      "empty items list returns zero counts",
			payload:   `{"items":[]}`,
			wantReady: 0,
			wantTotal: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ready, total, err := parseNodeReadiness([]byte(tc.payload))
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q; got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %q; want substring %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ready != tc.wantReady {
				t.Errorf("ready = %d; want %d", ready, tc.wantReady)
			}
			if total != tc.wantTotal {
				t.Errorf("total = %d; want %d", total, tc.wantTotal)
			}
		})
	}
}
