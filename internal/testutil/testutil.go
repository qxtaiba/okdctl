// Package testutil provides fake-binary PATH stubs and slog capture
// helpers shared by test files across the repository. Nothing here is
// imported by production code.
package testutil

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

var (
	fakeBinMu   sync.Mutex
	fakeBinDirs = map[string]string{}
)

// InstallFakeBin installs an executable file named name containing script
// and prepends its directory to PATH for the test's duration. Each distinct
// (name, script) pair is written exactly once per test-binary run and the
// directory is reused across tests: macOS Gatekeeper charges ~0.4s for the
// first exec of every freshly written script, so per-test temp dirs made
// suites pay that scan hundreds of times. Per-test behavior differences
// must therefore flow through env vars (OC_EXIT_CODE, OC_ARGV_LOG, ...),
// never through per-test script edits. The shared directories are not
// removed on test completion — the leak is bounded by the number of
// distinct scripts per run and lands in the OS temp dir.
// It skips the test on Windows since fake binaries rely on POSIX sh.
func InstallFakeBin(t *testing.T, name, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake binaries rely on POSIX sh")
	}
	dir, err := fakeBinDir(name, script)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

func fakeBinDir(name, script string) (string, error) {
	fakeBinMu.Lock()
	defer fakeBinMu.Unlock()
	key := name + "\x00" + script
	if dir, ok := fakeBinDirs[key]; ok {
		return dir, nil
	}
	dir, err := os.MkdirTemp("", "okdctl-fakebin-")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil { //nolint:gosec // G306: fake test binary needs +x
		return "", err
	}
	fakeBinDirs[key] = dir
	return dir, nil
}

// ArgvLogEnv is the environment variable naming the file that fake binaries
// installed by InstallArgvLoggingBin append their argv to, and ArgvExitCodeEnv
// overrides their exit status (default 0). Tests that need per-invocation
// behavior beyond argv capture (state machines, per-arg failure) keep their
// own local scriptlet rather than parameterizing these.
const (
	ArgvLogEnv      = "TESTUTIL_ARGV_LOG"
	ArgvExitCodeEnv = "TESTUTIL_EXIT_CODE"
)

const argvLoggingScript = "#!/bin/sh\n" +
	`printf '%s\n' "$(basename "$0") $*" >> "$` + "TESTUTIL_ARGV_LOG" + `"` + "\n" +
	`exit "${` + "TESTUTIL_EXIT_CODE" + `:-0}"` + "\n"

// InstallArgvLoggingBin installs a fake binary named name that appends one line
// per invocation — "<basename> <space-joined args>" — to a shared per-test log
// file, then exits with $TESTUTIL_EXIT_CODE (default 0). It returns the log
// path; read it back with ReadArgvLog. Calling it more than once in a test
// (e.g. to fake both helm and oc) installs each binary against the same log
// file, so multi-binary tests get one ordered trail keyed by basename. Set
// ArgvExitCodeEnv via t.Setenv to make every fake exit non-zero.
func InstallArgvLoggingBin(t *testing.T, name string) string {
	t.Helper()
	logPath := os.Getenv(ArgvLogEnv)
	if logPath == "" {
		logPath = filepath.Join(t.TempDir(), "argv.log")
		t.Setenv(ArgvLogEnv, logPath)
	}
	InstallFakeBin(t, name, argvLoggingScript)
	return logPath
}

// ReadArgvLog returns the non-empty lines an InstallArgvLoggingBin fake wrote
// to path, one per invocation in call order. A missing file yields nil (the
// binary was never invoked) rather than a fatal, so callers can assert "no
// calls" without a stat dance.
func ReadArgvLog(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		t.Fatalf("read argv log: %v", err)
	}
	var lines []string
	for _, l := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if l != "" {
			lines = append(lines, l)
		}
	}
	return lines
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
