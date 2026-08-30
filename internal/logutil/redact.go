package logutil

import (
	"context"
	"log/slog"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

// Redacted is the placeholder value that replaces any secret-bearing attr
// before it leaves the log sink.
const Redacted = "[redacted]"

// secretKeyFragments are case-insensitive substrings marking a slog key's
// value as a secret candidate. Deliberately narrow: "key"/"auth" stay out to
// avoid over-redacting public_key/oauth_flow-style names. stderrScrubRe is
// built from this slice — add new fragments here, not there.
var secretKeyFragments = []string{
	"password", "token", "secret", "api_key", "apikey",
	"credential", "passphrase", "authorization",
}

// RedactHandler wraps an inner slog.Handler, rewriting attrs whose keys match
// secretKeyFragments to the Redacted sentinel, and rewriting credential-shaped
// values (*url.URL with userinfo, any Redacted() any implementer).
// InstallHandler wraps every facade sink in it, so no call site can opt out.
//
// Credential types (e.g. ProxmoxCredentials) MUST implement Redacted() any
// omitting all secret fields — the only protection for credentials logged
// under a benign key (slog.Any("creds", &ProxmoxCredentials{})); key-based
// redaction alone can't catch them.
type RedactHandler struct {
	inner slog.Handler
}

// NewRedactHandler wraps inner with secret-redaction middleware.
func NewRedactHandler(inner slog.Handler) *RedactHandler {
	return &RedactHandler{inner: inner}
}

// Enabled delegates to the wrapped handler.
func (h *RedactHandler) Enabled(ctx context.Context, lvl slog.Level) bool {
	return h.inner.Enabled(ctx, lvl)
}

// Handle rewrites every Attr on the record before delegating.
func (h *RedactHandler) Handle(ctx context.Context, r slog.Record) error { //nolint:gocritic // hugeParam: slog.Handler passes slog.Record by value
	redacted := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		redacted.AddAttrs(redactAttr(a))
		return true
	})
	return h.inner.Handle(ctx, redacted)
}

// WithAttrs propagates redaction to a logger derived via slog.With.
func (h *RedactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	safe := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		safe[i] = redactAttr(a)
	}
	return &RedactHandler{inner: h.inner.WithAttrs(safe)}
}

// WithGroup propagates redaction through slog.WithGroup.
func (h *RedactHandler) WithGroup(name string) slog.Handler {
	return &RedactHandler{inner: h.inner.WithGroup(name)}
}

// redactAttr returns a sanitized copy of a for logging.
func redactAttr(a slog.Attr) slog.Attr {
	if KeyIsSecret(a.Key) {
		return slog.String(a.Key, Redacted)
	}
	switch a.Value.Kind() {
	case slog.KindAny:
		return slog.Any(a.Key, redactAny(a.Value.Any()))
	case slog.KindGroup:
		attrs := a.Value.Group()
		safe := make([]any, 0, len(attrs)*2)
		for _, sub := range attrs {
			ra := redactAttr(sub)
			safe = append(safe, ra.Key, ra.Value)
		}
		return slog.Group(a.Key, safe...)
	case slog.KindString:
		if keyIsURLLike(a.Key) {
			return slog.String(a.Key, stripURLUserinfo(a.Value.String()))
		}
		return a
	default:
		return a
	}
}

// urlKeyFragments flags attrs whose string value may be a URL with userinfo —
// url/endpoint keys don't match KeyIsSecret, so a Proxmox endpoint with
// user:password@ would otherwise log verbatim.
var urlKeyFragments = []string{"url", "uri", "endpoint"}

// keyIsURLLike reports whether key names a URL-valued attr, using the same
// case-insensitive substring match as KeyIsSecret.
func keyIsURLLike(key string) bool {
	lower := strings.ToLower(key)
	return slices.ContainsFunc(urlKeyFragments, func(f string) bool {
		return strings.Contains(lower, f)
	})
}

// stripURLUserinfo parses s as a URL and drops the userinfo password
// (username kept), mirroring redactAny's *url.URL handling. Non-URL or
// userinfo-free strings pass through unchanged.
func stripURLUserinfo(s string) string {
	u, err := url.Parse(s)
	if err != nil || u.User == nil {
		return s
	}
	if _, hasPassword := u.User.Password(); !hasPassword {
		return s
	}
	u.User = url.User(u.User.Username())
	return u.String()
}

// redactAny handles concrete types whose zero-knowledge string form would
// leak credentials (ProxmoxCredentials, *url.URL with userinfo).
func redactAny(v any) any {
	switch vv := v.(type) {
	case *url.URL:
		if vv == nil || vv.User == nil {
			return v
		}
		clone := *vv
		clone.User = url.User(vv.User.Username())
		return &clone
	case url.URL:
		if vv.User == nil {
			return v
		}
		clone := vv
		clone.User = url.User(vv.User.Username())
		return clone
	case interface{ Redacted() any }:
		return vv.Redacted()
	default:
		return v
	}
}

// RedactableArgv is a named slice for process argv (os.Args[1:]); pass it as
// slog.Any("argv", RedactableArgv(...)) so RedactHandler routes it through
// Redacted() any instead of a pre-joined string. Scrubbing (before joining)
// replaces --key=value/key=value values and the value after a bare secret
// flag when the key matches KeyIsSecret.
type RedactableArgv []string

// Redacted scrubs credential-bearing tokens in the argv slice and returns
// the joined result as a string.
func (a RedactableArgv) Redacted() any {
	out := make([]string, len(a))
	for i := 0; i < len(a); i++ {
		out[i] = scrubArgvToken(a[i])
		if bareSecretFlag(a[i]) && i+1 < len(a) && !strings.HasPrefix(a[i+1], "-") {
			i++
			out[i] = Redacted
		}
	}
	return strings.Join(out, " ")
}

func scrubArgvToken(tok string) string {
	bare := strings.TrimPrefix(tok, "--")
	eq := strings.IndexByte(bare, '=')
	if eq < 0 {
		return tok
	}
	if !KeyIsSecret(bare[:eq]) {
		return tok
	}
	prefix := tok[:len(tok)-len(bare)+eq+1]
	return prefix + Redacted
}

// bareSecretFlag reports whether tok is a dash-prefixed flag with no =value
// whose name matches KeyIsSecret, meaning the next argv entry carries the
// secret.
func bareSecretFlag(tok string) bool {
	if !strings.HasPrefix(tok, "-") {
		return false
	}
	name := strings.TrimLeft(tok, "-")
	return !strings.Contains(name, "=") && KeyIsSecret(name)
}

// RedactableStderr is a named string type for subprocess stderr text; pass
// it as slog.Any("stderr", rs) to route it through RedactHandler's
// Redacted() any dispatch (KindAny) instead of an unbounded raw string —
// the handler emits at most a head/tail excerpt.
type RedactableStderr string

// stderrScrubRe matches credential key–value pairs in these shapes:
//
//	key=value            (shell env / provider diagnostics)
//	key: value           (YAML / HTTP-style headers)
//	"key": "value"       (JSON diagnostics; quoted values may contain spaces)
//	Authorization: Bearer <token>
//
// Shares secretKeyFragments' denylist. Over-redaction is acceptable; under-redaction is not.
var stderrScrubRe = regexp.MustCompile(
	`(?i)((?:` + strings.Join(secretKeyFragments, "|") + `)` +
		`(?:["']?\s*[:=]\s*(?:Bearer\s+)?))("[^"]*"|'[^']*'|\S+)`,
)

// jwtScrubRe matches bare JWTs — three dot-joined base64url segments whose
// header starts with eyJ (base64 of `{"`). These carry no adjacent key for
// stderrScrubRe to anchor on, so they get their own pass.
var jwtScrubRe = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`)

// scrubStderrText masks shape-based credential values: key-adjacent pairs
// (stderrScrubRe) and bare JWTs (jwtScrubRe). A secret in neither shape (bare
// base64, prose-embedded) is NOT detected — RedactableStderr.Redacted's
// head/tail truncation is the only bound on such text.
func scrubStderrText(s string) string {
	s = stderrScrubRe.ReplaceAllString(s, "${1}"+Redacted)
	return jwtScrubRe.ReplaceAllString(s, Redacted)
}

// ScrubSecrets masks credential-shaped values in s with the same shape-based
// coverage as scrubStderrText. Exported for writer-side scrubbing of streamed
// subprocess output (the install log debug-bundle archives) that never
// passes through RedactHandler.
func ScrubSecrets(s string) string {
	return scrubStderrText(s)
}

// Redacted scrubs credential shapes (see stderrScrubRe/jwtScrubRe), then caps
// output to a 200-byte head/tail excerpt past 400 chars — scrubbing before
// truncation so a window cut can't split a key and leak its value. Guarantee:
// key-shaped secrets and JWTs masked, output bounded — not all secrets detected.
func (s RedactableStderr) Redacted() any {
	const half = 200
	r := scrubStderrText(string(s))
	if len(r) <= half*2 {
		return r
	}
	return r[:half] + " … [truncated] … " + r[len(r)-half:]
}

// KeyIsSecret reports whether key looks like it names a credential — a
// case-insensitive substring match against the denylist fragments. Exported
// so redactConfig in cli/config.go can share the same rule without
// duplicating the fragment list.
func KeyIsSecret(key string) bool {
	if key == "" {
		return false
	}
	lower := strings.ToLower(key)
	return slices.ContainsFunc(secretKeyFragments, func(f string) bool {
		return strings.Contains(lower, f)
	})
}
