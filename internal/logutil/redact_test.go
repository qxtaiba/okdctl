package logutil

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/url"
	"strings"
	"testing"
)

func stderrThroughHandler(t *testing.T, raw string) string {
	t.Helper()
	var buf bytes.Buffer
	jsonLogger(&buf).Warn("subprocess failed", slog.Any("stderr", RedactableStderr(raw)))
	m := parseOne(t, &buf)
	got, ok := m["stderr"].(string)
	if !ok {
		t.Fatalf("stderr is %T; want string", m["stderr"])
	}
	return got
}

func TestRedactableStderr_ThroughHandler(t *testing.T) {
	const secret = "MY_PULL_SECRET_VALUE"
	cases := []struct {
		name        string
		raw         string
		wantExact   string
		wantAbsent  []string
		wantPresent []string
	}{
		{
			name:        "truncates long output",
			raw:         strings.Repeat("a", 250) + secret + strings.Repeat("b", 250),
			wantAbsent:  []string{secret},
			wantPresent: []string{"[truncated]"},
		},
		{
			name:      "short output passes through",
			raw:       "permission denied",
			wantExact: "permission denied",
		},
		{
			name:        "short credential is scrubbed not passed through",
			raw:         "token: supersecrettoken123",
			wantAbsent:  []string{"supersecrettoken123"},
			wantPresent: []string{Redacted},
		},
		{
			name: "long text credential in windows is scrubbed with truncation",
			raw: strings.Repeat("x", 190) + " token: headtoken " +
				strings.Repeat("z", 100) + " password=tailpass " + strings.Repeat("y", 190),
			wantAbsent:  []string{"headtoken", "tailpass"},
			wantPresent: []string{"[truncated]"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stderrThroughHandler(t, tc.raw)
			if tc.wantExact != "" && got != tc.wantExact {
				t.Errorf("stderr = %q; want %q", got, tc.wantExact)
			}
			for _, s := range tc.wantAbsent {
				if strings.Contains(got, s) {
					t.Errorf("credential %q found in log output: %q", s, got)
				}
			}
			for _, s := range tc.wantPresent {
				if !strings.Contains(got, s) {
					t.Errorf("expected %q in output: %q", s, got)
				}
			}
		})
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
		{
			name:    "credential= shape",
			input:   "aws_credential=AKIAEXAMPLEVALUE rejected",
			wantOut: "AKIAEXAMPLEVALUE",
		},
		{
			name:    "passphrase: shape",
			input:   "passphrase: opensesame did not unlock key",
			wantOut: "opensesame",
		},
		{
			name:    "bare JWT with no key shape",
			input:   "auth failed for eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJvcGVyYXRvciJ9.c2lnbmF0dXJlLWJ5dGVz in request",
			wantIn:  "auth failed for",
			wantOut: "eyJzdWIiOiJvcGVyYXRvciJ9",
		},
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

func TestKeyIsSecret(t *testing.T) {
	cases := []struct {
		key  string
		want bool
	}{
		{"password", true},
		{"proxmox_ve_api_token", true},
		{"client_secret", true},
		{"api_key", true},
		{"apikey", true},
		{"aws_credentials", true},
		{"ssh_passphrase", true},
		{"authorization", true},
		{"Authorization", true},
		{"public_key", false},
		{"oauth_flow", false},
		{"endpoint", false},
		{"username", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := KeyIsSecret(tc.key); got != tc.want {
			t.Errorf("KeyIsSecret(%q) = %v; want %v", tc.key, got, tc.want)
		}
	}
}

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

func TestRedactHandler_SecretKeysRedacted(t *testing.T) {
	for _, key := range []string{"password", "PASSWORD", "PaSsWoRd", "api_token"} {
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

// Covers the plain-string path, which never reaches redactAny's *url.URL case.
func TestRedactHandler_URLStringUserinfoStripped(t *testing.T) {
	cases := []struct {
		name string
		key  string
		in   string
		want string
	}{
		{"endpoint key strips password", "endpoint", "https://root:hunter2@pve:8006", "https://root@pve:8006"},
		{"url key strips password", "url", "https://alice:s3cret@host/path", "https://alice@host/path"},
		{"uri key strips password", "proxmox_uri", "https://u:p@h", "https://u@h"},
		{"userinfo-free endpoint unchanged", "endpoint", "https://pve:8006", "https://pve:8006"},
		{"non-url key unchanged", "message", "https://u:p@h", "https://u:p@h"},
		{"non-url value unchanged", "endpoint", "just some text", "just some text"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := redactAttr(slog.String(tc.key, tc.in))
			if got := out.Value.String(); got != tc.want {
				t.Errorf("redactAttr(%q=%q) = %q; want %q", tc.key, tc.in, got, tc.want)
			}
		})
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

func TestRedactableArgv_Redacted(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		want string
	}{
		{
			name: "plain tokens pass through",
			argv: []string{"install", "--verbose", "--output=json"},
			want: "install --verbose --output=json",
		},
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

// redactor implements Redacted() any for redactAny's dispatch test.
type redactor struct{ public string }

func (r redactor) Redacted() any { return r.public }

func TestRedactHandler_RedactedInterfaceHonored(t *testing.T) {
	v := redactor{public: "safe-value"}
	var buf bytes.Buffer
	jsonLogger(&buf).Info("msg", slog.Any("obj", v))
	m := parseOne(t, &buf)
	if got, ok := m["obj"].(string); !ok || got != "safe-value" {
		t.Errorf("obj = %v; want %q", m["obj"], "safe-value")
	}
}

// credStub mirrors ProxmoxCredentials: a secret field plus Redacted() any that omits it.
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

func TestRedactHandler_WithAttrsRedacts(t *testing.T) {
	var buf bytes.Buffer
	logger := jsonLogger(&buf).With("password", "hunter2")
	logger.Info("msg")
	m := parseOne(t, &buf)
	if got := m["password"]; got != Redacted {
		t.Errorf("password = %v; want %q", got, Redacted)
	}
}

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
