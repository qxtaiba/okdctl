package logutil

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/url"
	"strings"
	"testing"
)

func TestRedactableStderr_TruncatesLongOutput(t *testing.T) {
	const secret = "MY_PULL_SECRET_VALUE"
	prefix := strings.Repeat("a", 250)
	suffix := strings.Repeat("b", 250)
	raw := prefix + secret + suffix

	var buf bytes.Buffer
	jsonLogger(&buf).Warn("subprocess failed", slog.Any("stderr", RedactableStderr(raw)))
	m := parseOne(t, &buf)
	got, ok := m["stderr"].(string)
	if !ok {
		t.Fatalf("stderr is %T; want string", m["stderr"])
	}
	if strings.Contains(got, secret) {
		t.Errorf("raw secret found in log output: %q", got)
	}
	if !strings.Contains(got, "[truncated]") {
		t.Errorf("expected truncation marker in output: %q", got)
	}
}

func TestRedactableStderr_ShortOutputPassesThrough(t *testing.T) {
	const msg = "permission denied"
	var buf bytes.Buffer
	jsonLogger(&buf).Warn("subprocess failed", slog.Any("stderr", RedactableStderr(msg)))
	m := parseOne(t, &buf)
	if got := m["stderr"]; got != msg {
		t.Errorf("stderr = %v; want %q", got, msg)
	}
}

func TestRedactableStderr_ContentScrub(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantIn  string
		wantOut string
	}{
		{
			name:    "key=value shape",
			input:   "error: password=hunter2 during auth",
			wantOut: "hunter2",
		},
		{
			name:    "key: value shape",
			input:   "token: abc123 is invalid",
			wantOut: "abc123",
		},
		{
			name:    "Authorization Bearer shape",
			input:   "Authorization: Bearer eyJhbGciOiJSUzI1",
			wantOut: "eyJhbGciOiJSUzI1",
		},
		{
			name:    "secret= shape",
			input:   "client_secret=topsecretvalue",
			wantOut: "topsecretvalue",
		},
		{
			name:   "unrelated text unchanged",
			input:  "connection refused: dial tcp 10.0.0.1:6443",
			wantIn: "connection refused",
		},
		{
			name:    "case-insensitive key match",
			input:   "PASSWORD=MySecretPass",
			wantOut: "MySecretPass",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := scrubStderrText(tc.input)
			if tc.wantOut != "" && strings.Contains(got, tc.wantOut) {
				t.Errorf("scrubStderrText(%q) = %q; credential %q must be absent", tc.input, got, tc.wantOut)
			}
			if tc.wantIn != "" && !strings.Contains(got, tc.wantIn) {
				t.Errorf("scrubStderrText(%q) = %q; expected %q to remain", tc.input, got, tc.wantIn)
			}
			if tc.wantOut != "" && !strings.Contains(got, Redacted) {
				t.Errorf("scrubStderrText(%q) = %q; expected Redacted sentinel", tc.input, got)
			}
		})
	}
}

func TestRedactableStderr_ShortCredentialIsScrubbedNotPassedThrough(t *testing.T) {
	raw := "token: supersecrettoken123"
	var buf bytes.Buffer
	jsonLogger(&buf).Warn("subprocess failed", slog.Any("stderr", RedactableStderr(raw)))
	m := parseOne(t, &buf)
	got, ok := m["stderr"].(string)
	if !ok {
		t.Fatalf("stderr is %T; want string", m["stderr"])
	}
	if strings.Contains(got, "supersecrettoken123") {
		t.Errorf("raw token found in log output: %q", got)
	}
	if !strings.Contains(got, Redacted) {
		t.Errorf("expected Redacted sentinel in output: %q", got)
	}
}

func TestRedactableStderr_LongTextCredentialInWindowsIsScrubbedWithTruncation(t *testing.T) {
	head := strings.Repeat("x", 190) + " token: headtoken "
	tail := " password=tailpass " + strings.Repeat("y", 190)
	filler := strings.Repeat("z", 100)
	raw := head + filler + tail
	var buf bytes.Buffer
	jsonLogger(&buf).Warn("subprocess failed", slog.Any("stderr", RedactableStderr(raw)))
	m := parseOne(t, &buf)
	got, ok := m["stderr"].(string)
	if !ok {
		t.Fatalf("stderr is %T; want string", m["stderr"])
	}
	if strings.Contains(got, "headtoken") {
		t.Errorf("head credential token found in output: %q", got)
	}
	if strings.Contains(got, "tailpass") {
		t.Errorf("tail credential password found in output: %q", got)
	}
	if !strings.Contains(got, "[truncated]") {
		t.Errorf("expected truncation marker in output: %q", got)
	}
}

// jsonLogger builds a RedactHandler over a JSON sink backed by buf.
func jsonLogger(buf *bytes.Buffer) *slog.Logger {
	inner := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(NewRedactHandler(inner))
}

func parseOne(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("json.Unmarshal: %v (raw: %q)", err, buf.String())
	}
	return m
}

// TestRedactHandler_PasswordKeyIsRedacted covers Fix case (1): the
// password key emits the [redacted] sentinel in place of the value.
func TestRedactHandler_PasswordKeyIsRedacted(t *testing.T) {
	var buf bytes.Buffer
	jsonLogger(&buf).Info("msg", "password", "hunter2")
	m := parseOne(t, &buf)
	if got := m["password"]; got != Redacted {
		t.Errorf("password = %v; want %q", got, Redacted)
	}
}

// TestRedactHandler_CaseInsensitiveKeys covers Fix case (2): mixed-case
// secret-key fragments still match.
func TestRedactHandler_CaseInsensitiveKeys(t *testing.T) {
	for _, key := range []string{"PASSWORD", "PaSsWoRd", "api_token"} {
		t.Run(key, func(t *testing.T) {
			var buf bytes.Buffer
			jsonLogger(&buf).Info("msg", key, "s3cr3t")
			m := parseOne(t, &buf)
			if got := m[key]; got != Redacted {
				t.Errorf("key %q = %v; want %q", key, got, Redacted)
			}
		})
	}
}

// TestRedactHandler_GroupRecursion covers Fix case (3): redaction
// descends into nested slog.Group attrs.
func TestRedactHandler_GroupRecursion(t *testing.T) {
	var buf bytes.Buffer
	jsonLogger(&buf).Info("msg", slog.Group("creds", "password", "hunter2"))
	m := parseOne(t, &buf)
	creds, ok := m["creds"].(map[string]any)
	if !ok {
		t.Fatalf("creds not a JSON object: %T %v", m["creds"], m["creds"])
	}
	if got := creds["password"]; got != Redacted {
		t.Errorf("creds.password = %v; want %q", got, Redacted)
	}
}

// TestRedactHandler_URLUserinfoStripped covers Fix case (4): *url.URL
// userinfo loses its password but keeps the username.
func TestRedactHandler_URLUserinfoStripped(t *testing.T) {
	u, err := url.Parse("https://alice:hunter2@host/path")
	if err != nil {
		t.Fatal(err)
	}
	out := redactAttr(slog.Any("endpoint", u))
	got, ok := out.Value.Any().(*url.URL)
	if !ok {
		t.Fatalf("redactAttr returned non-*url.URL: %T", out.Value.Any())
	}
	if got.User.Username() != "alice" {
		t.Errorf("username = %q; want %q", got.User.Username(), "alice")
	}
	if _, hasPwd := got.User.Password(); hasPwd {
		t.Errorf("password must be absent after redaction")
	}
}

// TestRedactHandler_NilURLPassesThrough covers Fix case (5): a typed-nil
// *url.URL must not panic and must pass through as a typed-nil *url.URL.
func TestRedactHandler_NilURLPassesThrough(t *testing.T) {
	defer func() {
		if p := recover(); p != nil {
			t.Errorf("panic on nil *url.URL: %v", p)
		}
	}()
	var u *url.URL
	out := redactAttr(slog.Any("endpoint", u))
	got, ok := out.Value.Any().(*url.URL)
	if !ok {
		t.Fatalf("redactAttr returned non-*url.URL: %T", out.Value.Any())
	}
	if got != nil {
		t.Errorf("nil *url.URL should pass through as nil, got %v", got)
	}
}

func TestRedactableArgv_PlainTokensPassThrough(t *testing.T) {
	argv := RedactableArgv([]string{"install", "--verbose", "--output=json"})
	got, ok := argv.Redacted().(string)
	if !ok {
		t.Fatalf("Redacted() returned %T, want string", argv.Redacted())
	}
	if got != "install --verbose --output=json" {
		t.Errorf("argv = %q; want %q", got, "install --verbose --output=json")
	}
}

func TestRedactableArgv_SecretTokenScrubbed(t *testing.T) {
	cases := []struct {
		tok  string
		want string
	}{
		{"--token=abc123", "--token=" + Redacted},
		{"--password=hunter2", "--password=" + Redacted},
		{"token=abc123", "token=" + Redacted},
		{"--api_key=xyz", "--api_key=" + Redacted},
		{"--verbose", "--verbose"},
		{"install", "install"},
	}
	for _, tc := range cases {
		t.Run(tc.tok, func(t *testing.T) {
			if got := scrubArgvToken(tc.tok); got != tc.want {
				t.Errorf("scrubArgvToken(%q) = %q; want %q", tc.tok, got, tc.want)
			}
		})
	}
}

func TestRedactableArgv_SpaceSeparatedSecretScrubbed(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		want string
	}{
		{
			name: "double-dash flag with following value",
			argv: []string{"install", "--token", "abc123"},
			want: "install --token " + Redacted,
		},
		{
			name: "single-dash flag with following value",
			argv: []string{"install", "-token", "abc123"},
			want: "install -token " + Redacted,
		},
		{
			name: "secret flag as last arg",
			argv: []string{"install", "--token"},
			want: "install --token",
		},
		{
			name: "following arg is another flag",
			argv: []string{"install", "--token", "--verbose"},
			want: "install --token --verbose",
		},
		{
			name: "non-secret flag value passes through",
			argv: []string{"install", "--output", "json"},
			want: "install --output json",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := RedactableArgv(tc.argv).Redacted().(string)
			if !ok {
				t.Fatalf("Redacted() returned non-string")
			}
			if got != tc.want {
				t.Errorf("Redacted() = %q; want %q", got, tc.want)
			}
		})
	}
}

func TestRedactableArgv_HandlerDispatch(t *testing.T) {
	argv := RedactableArgv([]string{"install", "--token=secret99", "--verbose"})
	var buf bytes.Buffer
	jsonLogger(&buf).Info("okdctl: started", slog.Any("argv", argv))
	m := parseOne(t, &buf)
	got, ok := m["argv"].(string)
	if !ok {
		t.Fatalf("argv is %T; want string", m["argv"])
	}
	if strings.Contains(got, "secret99") {
		t.Errorf("secret value found in log output: %q", got)
	}
	if !strings.Contains(got, Redacted) {
		t.Errorf("expected Redacted sentinel in argv output: %q", got)
	}
}

// redactor satisfies the inline interface{ Redacted() any } that
// redactAny matches at redact.go:107.
type redactor struct{ public string }

func (r redactor) Redacted() any { return r.public }

// TestRedactHandler_RedactedInterfaceHonored covers Fix case (6).
func TestRedactHandler_RedactedInterfaceHonored(t *testing.T) {
	v := redactor{public: "safe-value"}
	var buf bytes.Buffer
	jsonLogger(&buf).Info("msg", slog.Any("obj", v))
	m := parseOne(t, &buf)
	if got, ok := m["obj"].(string); !ok || got != "safe-value" {
		t.Errorf("obj = %v; want %q", m["obj"], "safe-value")
	}
}

// credStub mirrors the production ProxmoxCredentials shape: a struct with a
// secret field, plus Redacted() any that omits it.
type credStub struct {
	Endpoint string
	Password string
}

type safeCredStub struct{ Endpoint string }

func (c *credStub) Redacted() any {
	if c == nil {
		return nil
	}
	return safeCredStub{Endpoint: c.Endpoint}
}

// TestRedactHandler_StructFieldsStrippedViaBenignKey covers the failure mode
// from obs:41a9d4eb: a credential struct passed under a benign key like
// "creds" must have its secret fields stripped via Redacted().
func TestRedactHandler_StructFieldsStrippedViaBenignKey(t *testing.T) {
	cred := &credStub{Endpoint: "https://host:8006", Password: "hunter2"}
	var buf bytes.Buffer
	jsonLogger(&buf).Info("msg", slog.Any("creds", cred))
	m := parseOne(t, &buf)
	credsVal, ok := m["creds"].(map[string]any)
	if !ok {
		t.Fatalf("creds is %T %v, want JSON object", m["creds"], m["creds"])
	}
	if _, hasPassword := credsVal["Password"]; hasPassword {
		t.Errorf("Password field must be absent after Redacted() strip, got %v", credsVal)
	}
	if ep, ok := credsVal["Endpoint"].(string); !ok || ep != "https://host:8006" {
		t.Errorf("Endpoint = %v; want %q", credsVal["Endpoint"], "https://host:8006")
	}
}

func TestRedactHandler_NilRedactablePassesThrough(t *testing.T) {
	defer func() {
		if p := recover(); p != nil {
			t.Errorf("panic on nil credStub: %v", p)
		}
	}()
	var cred *credStub
	out := redactAttr(slog.Any("creds", cred))
	if got := out.Value.Any(); got != nil {
		t.Errorf("nil credStub Redacted() should return nil, got %v", got)
	}
}

// TestRedactHandler_WithAttrsRedacts covers Fix case (7): attrs added
// via logger.With are redacted before reaching the inner sink.
func TestRedactHandler_WithAttrsRedacts(t *testing.T) {
	var buf bytes.Buffer
	logger := jsonLogger(&buf).With("password", "hunter2")
	logger.Info("msg")
	m := parseOne(t, &buf)
	if got := m["password"]; got != Redacted {
		t.Errorf("password = %v; want %q", got, Redacted)
	}
}

// TestRedactHandler_WithGroupPropagates covers Fix case (8): keys
// inside a group from WithGroup still get redacted.
func TestRedactHandler_WithGroupPropagates(t *testing.T) {
	var buf bytes.Buffer
	logger := jsonLogger(&buf).WithGroup("grp")
	logger.Info("msg", "password", "hunter2")
	m := parseOne(t, &buf)
	grp, ok := m["grp"].(map[string]any)
	if !ok {
		t.Fatalf("grp not a JSON object: %T %v", m["grp"], m["grp"])
	}
	if got := grp["password"]; got != Redacted {
		t.Errorf("grp.password = %v; want %q", got, Redacted)
	}
}

func TestRedactableStderr_QuotedShapes(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantOut string
	}{
		{
			name:    "json quoted key and value",
			input:   `{"password": "hunter2"}`,
			wantOut: "hunter2",
		},
		{
			name:    "json compact quoted key",
			input:   `"token":"abc123"`,
			wantOut: "abc123",
		},
		{
			name:    "quoted value with spaces",
			input:   `password="my secret phrase"`,
			wantOut: "my secret phrase",
		},
		{
			name:    "single-quoted value with spaces",
			input:   `api_key='spaced out value'`,
			wantOut: "spaced out value",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := scrubStderrText(tc.input)
			if strings.Contains(got, tc.wantOut) {
				t.Errorf("scrubStderrText(%q) = %q; credential %q must be absent", tc.input, got, tc.wantOut)
			}
			if !strings.Contains(got, Redacted) {
				t.Errorf("scrubStderrText(%q) = %q; expected Redacted sentinel", tc.input, got)
			}
		})
	}
}
