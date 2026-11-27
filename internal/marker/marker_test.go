package marker

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type testPayload struct {
	Envelope

	Kind string `json:"kind"`
}

var testFile = File{Label: "test", Version: "v2"}

func TestFileWriteReadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "marker.json")
	if err := testFile.Write(path, &testPayload{Kind: "alpha"}, "prod"); err != nil {
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
	if err := testFile.Read(path, &got); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Kind != "alpha" || got.SchemaVersion != "v2" || got.ClusterName != "prod" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestFileReadErrors(t *testing.T) {
	tests := []struct {
		name    string
		seed    string
		wantIs  error
		wantMsg string
	}{
		{name: "missing file", wantIs: os.ErrNotExist, wantMsg: "read test marker"},
		{name: "corrupt json", seed: "{broken", wantMsg: "parse test marker"},
		{name: "unknown version", seed: `{"schema_version":"v99","kind":"alpha","cluster_name":"prod"}`, wantIs: ErrSchemaVersion, wantMsg: `"v99", want "v2"`},
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
			err := testFile.Read(path, &got)
			if err == nil {
				t.Fatal("Read accepted the marker")
			}
			if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
				t.Errorf("err = %v; want errors.Is %v", err, tc.wantIs)
			}
			if !strings.Contains(err.Error(), tc.wantMsg) || !strings.Contains(err.Error(), path) {
				t.Errorf("err = %q; want it to contain %q and the path", err, tc.wantMsg)
			}
		})
	}
}
