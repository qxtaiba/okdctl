package logutil

import (
	"context"
	"log/slog"
	"net/url"
	"strings"
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
// look like credentials — ProxmoxCredentials, byte slices keyed with
// password/token/secret, or *url.URL values carrying userinfo. Install via
// tui.SimpleLogger so every slog caller inherits the sweep without touching
// call sites.
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
	if keyIsSecret(a.Key) {
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

// keyIsSecret reports whether the slog key looks like it names a credential.
func keyIsSecret(key string) bool {
	if key == "" {
		return false
	}
	lower := strings.ToLower(key)
	for _, f := range secretKeyFragments {
		if strings.Contains(lower, f) {
			return true
		}
	}
	return false
}
