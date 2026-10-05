package logutil

import (
	"bytes"
	"context"
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

// Locks the redaction guarantee for both the facade and SimpleLogger paths.
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

func TestProgressBarsEnabled_DefaultsToDisabled(t *testing.T) {
	prev := ProgressBarsEnabled()
	t.Cleanup(func() { SetProgressBarsEnabled(prev) })

	if ProgressBarsEnabled() {
		t.Fatal("progress bars must default to disabled; only configureLogging enables them")
	}
	SetProgressBarsEnabled(true)
	if !ProgressBarsEnabled() {
		t.Fatal("SetProgressBarsEnabled(true) did not take effect")
	}
}

func TestRedirectDivertsAndRestores(t *testing.T) {
	installed := installBuffer(t)

	var diverted bytes.Buffer
	restore := Redirect(&diverted)
	Info("while the tui owns the terminal")
	restore()
	Info("after the tui released it")

	if !strings.Contains(diverted.String(), "while the tui owns the terminal") {
		t.Errorf("redirected sink is missing the line written during the redirect:\n%s", diverted.String())
	}
	if strings.Contains(installed.String(), "while the tui owns the terminal") {
		t.Errorf("the installed sink must see nothing during a redirect:\n%s", installed.String())
	}
	if !strings.Contains(installed.String(), "after the tui released it") {
		t.Errorf("restore must reinstate the previous sink:\n%s", installed.String())
	}
}

func TestRedirectKeepsRedaction(t *testing.T) {
	installBuffer(t)

	var diverted bytes.Buffer
	restore := Redirect(&diverted)
	Info("probing host", LF("password", "s3cret-bytes"))
	restore()

	if strings.Contains(diverted.String(), "s3cret-bytes") {
		t.Errorf("a redirected sink must still be wrapped in RedactHandler:\n%s", diverted.String())
	}
}

// recordingHandler captures the records reaching it, the shape RedirectHandler
// exists for: a TUI rendering the log stream itself needs the records, not the
// bytes a text handler would have encoded them into.
type recordingHandler struct {
	records []slog.Record
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error { //nolint:gocritic // hugeParam: slog.Handler passes slog.Record by value
	h.records = append(h.records, r)
	return nil
}

func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(string) slog.Handler      { return h }

func TestRedirectHandlerDivertsRecordsAndRestores(t *testing.T) {
	installed := installBuffer(t)

	rec := &recordingHandler{}
	restore := RedirectHandler(rec)
	Info("while the tui owns the terminal")
	restore()
	Info("after the tui released it")

	if len(rec.records) != 1 || rec.records[0].Message != "while the tui owns the terminal" {
		t.Fatalf("handler captured %+v, want the one diverted record", rec.records)
	}
	if strings.Contains(installed.String(), "while the tui owns the terminal") {
		t.Errorf("the installed sink must see nothing during a redirect:\n%s", installed.String())
	}
	if !strings.Contains(installed.String(), "after the tui released it") {
		t.Errorf("restore must reinstate the previous sink:\n%s", installed.String())
	}
}

func TestRedirectHandlerKeepsRedaction(t *testing.T) {
	installBuffer(t)

	rec := &recordingHandler{}
	restore := RedirectHandler(rec)
	Info("probing host", LF("password", "s3cret-bytes"))
	restore()

	if len(rec.records) != 1 {
		t.Fatalf("handler captured %d records, want 1", len(rec.records))
	}
	rec.records[0].Attrs(func(a slog.Attr) bool {
		if strings.Contains(a.Value.String(), "s3cret-bytes") {
			t.Errorf("a redirected handler must still sit inside RedactHandler: %s=%s", a.Key, a.Value)
		}
		return true
	})
}
