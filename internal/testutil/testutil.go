// Package testutil provides fake-binary PATH stubs and slog capture
// helpers shared by test files across the repository. Nothing here is
// imported by production code.
package testutil

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// InstallFakeBin writes script to a temp directory as an executable file
// named name and prepends that directory to PATH for the test's duration.
// It skips the test on Windows since fake binaries rely on POSIX sh.
func InstallFakeBin(t *testing.T, name, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake binaries rely on POSIX sh")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil { //nolint:gosec // G306: fake test binary needs +x
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

// CaptureHandler is an slog.Handler that records every emitted Record so
// tests can assert on level, message, and attrs. The zero value is ready
// to use.
type CaptureHandler struct {
	Records []slog.Record
	attrs   []slog.Attr
}

// Enabled always returns true so every record reaches Handle.
func (h *CaptureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

// Handle appends r, plus any attrs bound via WithAttrs, to Records.
func (h *CaptureHandler) Handle(_ context.Context, r slog.Record) error { //nolint:gocritic // hugeParam: slog.Handler interface requires value receiver
	r.AddAttrs(h.attrs...)
	h.Records = append(h.Records, r)
	return nil
}

// WithAttrs returns a handler that stamps attrs onto every future record,
// mirroring slog.Logger.With.
func (h *CaptureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &CaptureHandler{Records: h.Records, attrs: merged}
}

// WithGroup returns h unchanged; group scoping is not modeled.
func (h *CaptureHandler) WithGroup(_ string) slog.Handler { return h }

// Last returns the most recently captured record, or false if none exist.
func (h *CaptureHandler) Last() (slog.Record, bool) {
	if len(h.Records) == 0 {
		return slog.Record{}, false
	}
	return h.Records[len(h.Records)-1], true
}

// CountLevel returns the number of captured records emitted at level.
func (h *CaptureHandler) CountLevel(level slog.Level) int {
	n := 0
	for i := range h.Records {
		if h.Records[i].Level == level {
			n++
		}
	}
	return n
}

// HasLevel reports whether any captured record was emitted at level.
func (h *CaptureHandler) HasLevel(level slog.Level) bool {
	return h.CountLevel(level) > 0
}
