package config

// SecretBytes wraps a credential as []byte so the backing array can be
// overwritten with Zeroize once consumed. A Go string cannot be zeroized:
// its backing bytes remain on the heap until GC. SecretBytes lets the
// wizard store the captured credential in a mutable buffer and lets the
// deploy/destroy defer chain wipe it when done.
type SecretBytes struct {
	b []byte
}

// Set copies the string bytes into an owned []byte. Any previous backing
// array is zeroized first so a re-Set call does not leak the old secret.
// The argument string itself still lives on the heap until GC — the wizard
// input pipeline is the inherent capture boundary.
//
// Authorised production caller: internal/tui/wizard/steps (password field
// ConfigSet callback). Tests may call Set to seed fixture state. No other
// production package should call Set; the credentials layer reads secrets
// from env vars and assigns []byte directly to ProxmoxCredentials.
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

// Redacted satisfies the interface that logutil.RedactHandler detects
// for opaque-typed values.
func (s SecretBytes) Redacted() any {
	return "[redacted]"
}
