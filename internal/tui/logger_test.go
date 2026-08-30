package tui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/logutil"
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
	var buf bytes.Buffer
	configureBuf(t, &buf)

	SetRunID("run-42")
	buf.Reset()
	logutil.Info("after set run id", logutil.LF("password", "secret-pw"))
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
