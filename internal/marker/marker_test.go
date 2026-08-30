package marker

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

type testPayload struct {
	Envelope

	Kind string `json:"kind"`
}

var testFile = File{Label: "test", Version: "v2"}

func TestFileWriteReadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "marker.json")
	if err := testFile.Write(path, &testPayload{Kind: "alpha"}, "run-1", "prod"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("perm = %o; want 600", perm)
	}

	var got testPayload
	found, err := testFile.Read(path, &got)
	if err != nil || !found {
		t.Fatalf("Read = (%v, %v); want (true, nil)", found, err)
	}
	if got.Kind != "alpha" || got.SchemaVersion != "v2" || got.RunID != "run-1" || got.ClusterName != "prod" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if got.Timestamp.IsZero() {
		t.Error("Timestamp must be stamped on write")
	}
}

func TestFileReadAbsentAndError(t *testing.T) {
	tests := []struct {
		name    string
		seed    string // "" = no file on disk
		wantErr bool
	}{
		{name: "missing file treated absent"},
		{name: "corrupt JSON errors", seed: "{broken", wantErr: true},
		{name: "unknown version treated absent", seed: `{"schema_version":"v99","kind":"alpha","cluster_name":"prod"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "marker.json")
			if tc.seed != "" {
				if err := os.WriteFile(path, []byte(tc.seed), 0o600); err != nil {
					t.Fatalf("seed: %v", err)
				}
			}
			var got testPayload
			found, err := testFile.Read(path, &got)
			if tc.wantErr {
				if err == nil {
					t.Fatal("want error, got nil")
				}
				return
			}
			if err != nil || found {
				t.Fatalf("Read = (%v, %v); want (false, nil)", found, err)
			}
		})
	}
}

func TestFileTrusted(t *testing.T) {
	tests := []struct {
		name        string
		markerName  string
		clusterName string
		want        bool
	}{
		{name: "matching name is trusted", markerName: "prod", clusterName: "prod", want: true},
		{name: "empty name is rejected", markerName: "", clusterName: "prod", want: false},
		{name: "mismatching name is rejected", markerName: "other", clusterName: "prod", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &testPayload{Envelope: Envelope{ClusterName: tc.markerName}}
			if got := testFile.Trusted(p, tc.clusterName); got != tc.want {
				t.Errorf("Trusted = %v; want %v", got, tc.want)
			}
		})
	}
}

func TestFileClear(t *testing.T) {
	path := filepath.Join(t.TempDir(), "marker.json")
	if err := testFile.Write(path, &testPayload{}, "r", "c"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := testFile.Clear(path); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("marker still present after Clear: %v", err)
	}
	if err := testFile.Clear(path); err != nil {
		t.Fatalf("Clear on absent file: %v", err)
	}
}

func TestEnvelopeStale(t *testing.T) {
	fresh := Envelope{Timestamp: time.Now().UTC()}
	if fresh.Stale() {
		t.Error("fresh marker must not be stale")
	}
	old := Envelope{Timestamp: time.Now().UTC().Add(-StaleAfter - time.Hour)}
	if !old.Stale() {
		t.Error("week-old marker must be stale")
	}
}
