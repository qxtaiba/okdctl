package config

// SecretBytes wraps a credential as []byte, not string, so Zeroize can
// overwrite it before GC. The wizard fills it on capture; deploy/destroy
// wipes it via defer.
type SecretBytes struct {
	b []byte
}

// Set copies v into an owned []byte, zeroizing any prior backing array first
// so a re-Set can't leak the old secret. Only the wizard's password-field
// callback calls this in production; internal/credentials assigns []byte directly.
func (s *SecretBytes) Set(v string) {
	clear(s.b)
	s.b = []byte(v)
}

// Zeroize overwrites the backing array with zeros and nils the slice.
func (s *SecretBytes) Zeroize() {
	clear(s.b)
	s.b = nil
}

// Bytes returns the underlying slice. Callers must not retain the result
// past a Zeroize call on the same SecretBytes value.
func (s SecretBytes) Bytes() []byte {
	return s.b
}

// IsEmpty reports whether no secret bytes have been set.
func (s SecretBytes) IsEmpty() bool {
	return len(s.b) == 0
}

// String returns a redacted placeholder so %v / %s cannot leak the value.
func (s SecretBytes) String() string {
	return "[redacted]"
}

// Redacted satisfies the interface logutil.RedactHandler detects for opaque-typed values.
func (s SecretBytes) Redacted() any {
	return "[redacted]"
}
