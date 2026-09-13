package tui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

func TestStderrSlog_RedactsSecrets(t *testing.T) {
	var buf bytes.Buffer
	configureBuf(t, &buf)

	cases := []struct {
		key   string
		value string
	}{
		{"password", "hunter2"},
		{"PROXMOX_VE_PASSWORD", "hunter2"},
		{"api_token", "tok-abc"},
		{"api_key", "k-123"},
		{"apikey", "k-456"},
		{"bearer_token", "deadbeef"},
		{"session_secret", "shh"},
	}

	for _, tc := range cases {
		buf.Reset()
		logutil.Info("test message", logutil.LF(tc.key, tc.value))
		out := buf.String()
		if strings.Contains(out, tc.value) {
			t.Errorf("key=%s leaked value %q in tui.Info output:\n%s", tc.key, tc.value, out)
		}
		if !strings.Contains(out, "[redacted]") {
			t.Errorf("key=%s expected [redacted] marker in output:\n%s", tc.key, out)
		}
	}
}

func TestStderrSlog_NonSecretsPassThrough(t *testing.T) {
	var buf bytes.Buffer
	configureBuf(t, &buf)

	buf.Reset()
	logutil.Info("test message", logutil.LF("cluster", "prod"), logutil.LF("ip", "10.0.0.1"))
	out := buf.String()
	if !strings.Contains(out, "prod") {
		t.Errorf("non-secret 'cluster' value dropped: %s", out)
	}
	if !strings.Contains(out, "10.0.0.1") {
		t.Errorf("non-secret 'ip' value dropped: %s", out)
	}
}

func TestSetRunID_RefreshesRedactionWrapper(t *testing.T) {
	var stderrBuf, sinkBuf bytes.Buffer
	if err := ConfigureLoggers(LoggerConfig{Level: "debug", Format: "text", Stderr: &stderrBuf, Sink: &sinkBuf}); err != nil {
		t.Fatal(err)
	}
	logutil.InstallHandler(newStderrHandler())

	SetRunID("run-42")
	stderrBuf.Reset()
	sinkBuf.Reset()
	logutil.Info("after set run id", logutil.LF("password", "secret-pw"))
	stderrOut := stderrBuf.String()
	sinkOut := sinkBuf.String()

	if strings.Contains(stderrOut, "run_id") {
		t.Errorf("run_id leaked onto text stderr: %s", stderrOut)
	}
	if !strings.Contains(sinkOut, "run_id") {
		t.Errorf("run_id missing from sink: %s", sinkOut)
	}
	for _, out := range []string{stderrOut, sinkOut} {
		if strings.Contains(out, "secret-pw") {
			t.Errorf("redaction broke after SetRunID rebind: %s", out)
		}
		if !strings.Contains(out, "[redacted]") {
			t.Errorf("redaction marker missing post-SetRunID: %s", out)
		}
	}
}

func TestRunIDOnStderrWhenJSON(t *testing.T) {
	var stderrBuf bytes.Buffer
	if err := ConfigureLoggers(LoggerConfig{Level: "debug", Format: "json", Stderr: &stderrBuf}); err != nil {
		t.Fatal(err)
	}
	logutil.InstallHandler(newStderrHandler())

	SetRunID("run-json")
	stderrBuf.Reset()
	logutil.Info("json run id check")
	out := stderrBuf.String()
	if !strings.Contains(out, "run_id") {
		t.Errorf("run_id missing from json stderr output: %s", out)
	}
}

func TestBadgesShareMessageColumn(t *testing.T) {
	var buf bytes.Buffer
	configureBuf(t, &buf)

	buf.Reset()
	logutil.Info("hello")
	infoLine := tuitest.StripANSI(buf.String())

	buf.Reset()
	logutil.Error("boom")
	errorLine := tuitest.StripANSI(buf.String())

	if !strings.HasPrefix(infoLine, "[INFO]  ") {
		t.Fatalf("info line = %q, want prefix %q", infoLine, "[INFO]  ")
	}
	if !strings.HasPrefix(errorLine, "[ERROR] ") {
		t.Fatalf("error line = %q, want prefix %q", errorLine, "[ERROR] ")
	}
	if got, want := strings.Index(infoLine, "hello"), strings.Index(errorLine, "boom"); got != want {
		t.Fatalf("message columns differ: info starts at %d, error starts at %d", got, want)
	}
}
