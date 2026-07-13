package cluster

import "testing"

func TestParseClusterOperatorsAvailable(t *testing.T) {
	tests := []struct {
		name          string
		data          string
		wantAvailable int
		wantTotal     int
		wantErr       bool
	}{
		{
			name:          "empty list",
			data:          `{"items":[]}`,
			wantAvailable: 0,
			wantTotal:     0,
		},
		{
			name: "mixed availability",
			data: `{"items":[
				{"status":{"conditions":[{"type":"Available","status":"True"}]}},
				{"status":{"conditions":[{"type":"Available","status":"False"}]}},
				{"status":{"conditions":[{"type":"Degraded","status":"True"},{"type":"Available","status":"True"}]}}
			]}`,
			wantAvailable: 2,
			wantTotal:     3,
		},
		{
			name: "operator with no available condition counts to total only",
			data: `{"items":[
				{"status":{"conditions":[{"type":"Progressing","status":"True"}]}}
			]}`,
			wantAvailable: 0,
			wantTotal:     1,
		},
		{
			name:    "malformed json",
			data:    `{"items":`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			available, total, err := parseClusterOperatorsAvailable([]byte(tt.data))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if available != tt.wantAvailable || total != tt.wantTotal {
				t.Errorf("got %d/%d available; want %d/%d", available, total, tt.wantAvailable, tt.wantTotal)
			}
		})
	}
}
