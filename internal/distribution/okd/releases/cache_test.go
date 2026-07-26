package releases

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func cacheFetcher(t *testing.T) *OKDVersionFetcher {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SUDO_USER", "")
	return NewOKDVersionFetcher()
}

func TestDiskCache_RoundTrip(t *testing.T) {
	f := cacheFetcher(t)
	series := []OKDReleaseSeries{{Major: 4, Minor: 19, Latest: OKDVersion{Version: "4.19.0"}}}

	f.saveToDiskCache(series)
	got := f.loadFromDiskCache()
	if len(got) != 1 || got[0].Latest.Version != "4.19.0" {
		t.Fatalf("loadFromDiskCache = %+v; want the saved series back", got)
	}
}

// TestDiskCache_SchemaMismatchDiscarded locks the schema gate: a cache
// written by a binary with a different (or missing) schema field is treated
// like corruption and discarded, never served with a drifted shape.
func TestDiskCache_SchemaMismatchDiscarded(t *testing.T) {
	f := cacheFetcher(t)

	path, err := f.getCacheFilePath()
	if err != nil {
		t.Fatalf("getCacheFilePath: %v", err)
	}
	legacy := map[string]any{
		// no "schema" field — the pre-versioning layout
		"cached_at": time.Now(),
		"series":    []OKDReleaseSeries{{Major: 4, Minor: 19}},
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	if got := f.loadFromDiskCache(); got != nil {
		t.Fatalf("loadFromDiskCache = %+v; want nil for schemaless cache", got)
	}
}

func TestDiskCache_StaleDiscarded(t *testing.T) {
	f := cacheFetcher(t)
	f.saveToDiskCache([]OKDReleaseSeries{{Major: 4, Minor: 19}})
	f.diskCacheTTL = 0

	if got := f.loadFromDiskCache(); got != nil {
		t.Fatalf("loadFromDiskCache = %+v; want nil for expired cache", got)
	}
}
