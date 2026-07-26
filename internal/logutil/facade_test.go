package logutil

import (
	"bytes"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func installBuffer(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	InstallHandler(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	t.Cleanup(func() { InstallHandler(slog.NewTextHandler(os.Stderr, nil)) })
	return &buf
}

func TestFacade_LevelsAndFields(t *testing.T) {
	buf := installBuffer(t)

	Debug("dbg line", LF("k", "v1"))
	Info("info line", LF("k", "v2"))
	Warn("warn line", LF("k", "v3"))
	Error("error line", LF("k", "v4"))

	out := buf.String()
	for _, want := range []string{
		"level=DEBUG", `msg="dbg line"`, "k=v1",
		"level=INFO", `msg="info line"`, "k=v2",
		"level=WARN", `msg="warn line"`, "k=v3",
		"level=ERROR", `msg="error line"`, "k=v4",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

// TestInstallHandler_WrapsRedactHandler locks the redaction guarantee: any
// sink installed via InstallHandler is wrapped in RedactHandler, so secret-
// keyed attrs never reach it verbatim — through the facade or through a
// SimpleLogger handed to injected-logger call sites.
func TestInstallHandler_WrapsRedactHandler(t *testing.T) {
	buf := installBuffer(t)

	Info("facade creds", LF("password", "hunter2"))
	SimpleLogger().Info("injected creds", "api_token", "tok-abc")

	out := buf.String()
	for _, leaked := range []string{"hunter2", "tok-abc"} {
		if strings.Contains(out, leaked) {
			t.Errorf("secret %q leaked past InstallHandler:\n%s", leaked, out)
		}
	}
	if strings.Count(out, "[redacted]") != 2 {
		t.Errorf("expected 2 [redacted] markers:\n%s", out)
	}
}

func TestSimpleLogger_IsSnapshotOfInstallation(t *testing.T) {
	first := installBuffer(t)
	snap := SimpleLogger()

	var second bytes.Buffer
	InstallHandler(slog.NewTextHandler(&second, nil))

	snap.Info("to first sink")
	Info("to second sink")

	if !strings.Contains(first.String(), "to first sink") {
		t.Errorf("snapshot logger abandoned its sink: %q", first.String())
	}
	if !strings.Contains(second.String(), "to second sink") {
		t.Errorf("facade did not follow reinstall: %q", second.String())
	}
}

func TestRunID_RoundTrip(t *testing.T) {
	if got := RunID(); got != "" {
		t.Fatalf("RunID before SetRunID = %q, want empty", got)
	}
	SetRunID("run-42")
	if got := RunID(); got != "run-42" {
		t.Fatalf("RunID = %q, want run-42", got)
	}
}

func TestProgressBars_Toggle(t *testing.T) {
	if !ProgressBarsEnabled() {
		t.Fatal("progress bars should default to enabled")
	}
	SetProgressBarsEnabled(false)
	t.Cleanup(func() { SetProgressBarsEnabled(true) })
	if ProgressBarsEnabled() {
		t.Fatal("SetProgressBarsEnabled(false) not observed")
	}
}
