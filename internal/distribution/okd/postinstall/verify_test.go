package postinstall

import (
	"strings"
	"testing"
)

func TestParseNodeReadiness(t *testing.T) {
	cases := []struct {
		name      string
		payload   string
		wantReady int
		wantTotal int
		wantErr   string
	}{
		{
			name: "all three nodes ready",
			payload: `{
	"items": [
		{"metadata":{"name":"node-0"},"status":{"conditions":[{"type":"Ready","status":"True"}]}},
		{"metadata":{"name":"node-1"},"status":{"conditions":[{"type":"Ready","status":"True"}]}},
		{"metadata":{"name":"node-2"},"status":{"conditions":[{"type":"Ready","status":"True"}]}}
	]
}`,
			wantReady: 3,
			wantTotal: 3,
		},
		{
			// SchedulingDisabled nodes still expose Ready=True in their conditions;
			// the old strings.Contains text-parser misclassified these as not-ready.
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
