package credentials

// SecretBytes holds a credential as []byte so Zeroize can overwrite it
// before GC.
type SecretBytes struct {
	b []byte
}

// Set replaces the secret with v, zeroizing the prior backing array first.
func (s *SecretBytes) Set(v string) {
	clear(s.b)
	s.b = []byte(v)
}

// Zeroize overwrites the backing array with zeros and nils the slice.
func (s *SecretBytes) Zeroize() {
	clear(s.b)
	s.b = nil
}

// Bytes returns the backing slice; it is wiped by the next Set or Zeroize.
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

// Redacted is what logutil.RedactHandler logs in place of the value.
func (s SecretBytes) Redacted() any {
	return "[redacted]"
}
