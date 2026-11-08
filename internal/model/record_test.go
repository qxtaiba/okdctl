package model

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestNewInfraID(t *testing.T) {
	re := regexp.MustCompile(`^lab-[0-9a-f]{6}$`)
	a, b := NewInfraID("lab"), NewInfraID("lab")
	if !re.MatchString(a) || !re.MatchString(b) {
		t.Fatalf("NewInfraID = %q, %q; want %s", a, b, re)
	}
	if a == b {
		t.Errorf("NewInfraID repeated %q", a)
	}
}

func TestRecordRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "record.json")
	want := &Record{
		InfraID: "lab-a1b2c3", Cluster: "lab", Domain: "example.com",
		Created: time.Now().UTC().Truncate(time.Second), Release: "4.20.0",
		InputsHash: strings.Repeat("ab", 32),
	}
	if err := SaveRecord(path, want); err != nil {
		t.Fatalf("SaveRecord: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("perm = %o; want 600", perm)
	}
	got, err := LoadRecord(path)
	if err != nil {
		t.Fatalf("LoadRecord: %v", err)
	}
	if got.InfraID != want.InfraID || got.Cluster != want.Cluster || got.Domain != want.Domain ||
		got.Release != want.Release || got.InputsHash != want.InputsHash || !got.Created.Equal(want.Created) {
		t.Errorf("LoadRecord = %+v; want %+v", got, want)
	}
}

func TestLoadRecord_Errors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		seed   string
		wantIs error
		notIs  []error
	}{
		{name: "missing file", wantIs: os.ErrNotExist},
		{
			name:   "cluster name mismatch",
			seed:   `{"schema_version":"1","cluster_name":"other","infra_id":"lab-a1b2c3","cluster":"lab"}`,
			wantIs: ErrRecordMismatch,
		},
		{
			name:  "unknown schema version",
			seed:  `{"schema_version":"99","cluster_name":"lab","infra_id":"lab-a1b2c3","cluster":"lab"}`,
			notIs: []error{os.ErrNotExist, ErrRecordMismatch},
		},
		{name: "corrupt json", seed: `{broken`, notIs: []error{os.ErrNotExist, ErrRecordMismatch}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "record.json")
			if tc.seed != "" {
				if err := os.WriteFile(path, []byte(tc.seed), 0o600); err != nil {
					t.Fatalf("seed: %v", err)
				}
			}
			_, err := LoadRecord(path)
			if err == nil {
				t.Fatal("LoadRecord accepted the record")
			}
			if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
				t.Errorf("err = %v; want errors.Is %v", err, tc.wantIs)
			}
			for _, not := range tc.notIs {
				if errors.Is(err, not) {
					t.Errorf("err = %v; must not be %v", err, not)
				}
			}
		})
	}
}
