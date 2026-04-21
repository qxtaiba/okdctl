package tui

import (
	"bytes"
	"strings"
	"testing"
)

// TestStderrSlog_RedactsSecrets locks the RedactHandler wiring: an attr
// whose key names a credential (password/token/secret/api_key/apikey) must
// never appear verbatim in tui.X output, regardless of the value type.
func TestStderrSlog_RedactsSecrets(t *testing.T) {
	var buf bytes.Buffer
	if err := ConfigureLoggers("debug", "text", &buf, &buf, false); err != nil {
		t.Fatal(err)
	}
	// ConfigureLoggers only touches stdoutLogger/stderrLogger's output;
	// stderrSlog is already built from stderrLogger pointer so its output
	// follows SetOutput. Rebuild defensively to be explicit.
	stderrSlog = buildStderrSlog()

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
		Info("test message", LF(tc.key, tc.value))
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
	if err := ConfigureLoggers("debug", "text", &buf, &buf, false); err != nil {
		t.Fatal(err)
	}
	stderrSlog = buildStderrSlog()

	buf.Reset()
	Info("test message", LF("cluster", "prod"), LF("ip", "10.0.0.1"))
	out := buf.String()
	if !strings.Contains(out, "prod") {
		t.Errorf("non-secret 'cluster' value dropped: %s", out)
	}
	if !strings.Contains(out, "10.0.0.1") {
		t.Errorf("non-secret 'ip' value dropped: %s", out)
	}
}

func TestSetRunID_RefreshesRedactionWrapper(t *testing.T) {
	var buf bytes.Buffer
	if err := ConfigureLoggers("debug", "text", &buf, &buf, false); err != nil {
		t.Fatal(err)
	}
	stderrSlog = buildStderrSlog()

	SetRunID("run-42")
	buf.Reset()
	Info("after set run id", LF("password", "secret-pw"))
	out := buf.String()
	if !strings.Contains(out, "run_id") {
		t.Errorf("run_id not present after SetRunID: %s", out)
	}
	if strings.Contains(out, "secret-pw") {
		t.Errorf("redaction broke after SetRunID rebind: %s", out)
	}
	if !strings.Contains(out, "[redacted]") {
		t.Errorf("redaction marker missing post-SetRunID: %s", out)
	}
}
