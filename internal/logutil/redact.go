package logutil

import (
	"context"
	"log/slog"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
)

// Redacted is the placeholder value that replaces any secret-bearing attr
// before it leaves the log sink.
const Redacted = "[redacted]"

// secretKeyFragments are substrings that, when present in a slog key
// (case-insensitively), mark the value as a secret candidate. The list is
// deliberately narrow — log authors know not to put plaintext credentials
// in message text, so we only sweep structured attrs.
//
// Add new fragments here when a new credential domain appears (e.g.
// "private_key", "access_key"). Substring match: "token" already covers
// "auth_token", "bearer_token", "refresh_token", etc.
var secretKeyFragments = []string{"password", "token", "secret", "api_key", "apikey"}

// RedactHandler wraps an inner slog.Handler and rewrites attr values that
// look like credentials — *url.URL values carrying userinfo, or any type
// implementing Redacted() any — and rewrites attrs whose keys match
// secretKeyFragments to the Redacted sentinel. Install via tui.SimpleLogger
// so every slog caller inherits the sweep without touching call sites.
//
// Credential types (e.g. ProxmoxCredentials) MUST implement Redacted() any
// and return a struct that omits all secret fields. This is the only
// mechanism that protects credentials passed under a benign slog key such
// as slog.Any("creds", &credentials.ProxmoxCredentials{...}); key-based
// redaction alone cannot protect them.
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
func (h *RedactHandler) Handle(ctx context.Context, r slog.Record) error { //nolint:gocritic // hugeParam: slog.Handler interface requires value receiver
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
	default:
		return a
	}
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

// RedactableArgv is a named slice type for process argv (os.Args[1:]). Pass
// it as slog.Any("argv", logutil.RedactableArgv(os.Args[1:])) so RedactHandler
// dispatches through Redacted() any rather than logging a pre-joined string.
// Scrubbing runs before joining: any --key=value or key=value token whose key
// matches KeyIsSecret has its value replaced with the Redacted sentinel, and
// the value following a bare --key / -key secret flag (pflag's space-separated
// form) is replaced too, guarding against future flags that accept
// credentials.
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

// RedactableStderr is a named string type for subprocess stderr text. Pass
// it as slog.Any("stderr", rs) — the attr is then KindAny and routes through
// RedactHandler's Redacted() any dispatch instead of leaving the raw string
// unbounded. The handler emits at most a head/tail excerpt, preventing
// multi-kilobyte auth diagnostics or registry-config snippets from reaching
// the log sink verbatim.
type RedactableStderr string

// stderrScrubOnce guards one-time compilation of stderrScrubRe.
var stderrScrubOnce sync.Once

// stderrScrubRe matches credential key–value pairs in three shapes:
//
//	key=value            (shell env / provider diagnostics)
//	key: value           (YAML / HTTP-style headers)
//	"key": "value"       (JSON diagnostics; quoted values may contain spaces)
//	Authorization: Bearer <token>
//
// Covers secretKeyFragments plus "authorization". Over-redaction is
// acceptable; under-redaction is not.
var stderrScrubRe *regexp.Regexp

// scrubStderrText masks credential values in s using stderrScrubRe.
func scrubStderrText(s string) string {
	stderrScrubOnce.Do(func() {
		stderrScrubRe = regexp.MustCompile(
			`(?i)((?:password|token|secret|api_key|apikey|authorization)` +
				`(?:["']?\s*[:=]\s*(?:Bearer\s+)?))("[^"]*"|'[^']*'|\S+)`,
		)
	})
	return stderrScrubRe.ReplaceAllString(s, "${1}"+Redacted)
}

// Redacted masks credential values matching key=value, key: value, or
// Authorization: Bearer <token> shapes, then returns at most the first 200
// and last 200 bytes joined by a truncation marker when the scrubbed text
// exceeds 400 characters. Scrubbing runs before truncation so a window cut
// can never split a key and leave its value exposed in the retained tail.
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
