package config

import (
	"fmt"
	"strings"
	"testing"
)

func TestSecretBytes_SetZeroizesPriorBuffer(t *testing.T) {
	var s SecretBytes
	s.Set("first-secret")

	old := s.Bytes()
	oldCopy := make([]byte, len(old))
	copy(oldCopy, old)

	s.Set("second-secret")

	for i, b := range old {
		if b != 0 {
			t.Errorf("old backing[%d] = %d after Set; want 0 (was %q)", i, b, oldCopy[i])
		}
	}

	if got := string(s.Bytes()); got != "second-secret" {
		t.Errorf("Bytes() = %q after second Set; want second-secret", got)
	}
}

func TestSecretBytes_FmtVerbs(t *testing.T) {
	var s SecretBytes
	s.Set("super-secret-password")

	for _, verb := range []string{"%s", "%v", "%+v"} {
		rendered := fmt.Sprintf(verb, s)
		if strings.Contains(rendered, "super-secret-password") {
			t.Errorf("fmt verb %s leaked secret: %s", verb, rendered)
		}
		if !strings.Contains(rendered, "[redacted]") {
			t.Errorf("fmt verb %s missing [redacted]: %s", verb, rendered)
		}
	}
}

func TestSecretBytes_Redacted(t *testing.T) {
	var s SecretBytes
	s.Set("tok-abc")
	got := s.Redacted()
	if got != "[redacted]" {
		t.Errorf("Redacted() = %v; want [redacted]", got)
	}
}

// TestSecretBytes_SetBytesCopiesIndependently proves SetBytes does not
// retain the caller's slice: mutating (and later zeroizing) the caller's
// buffer after SetBytes must not change what SecretBytes stored.
func TestSecretBytes_SetBytesCopiesIndependently(t *testing.T) {
	var s SecretBytes
	caller := []byte("caller-owned-secret")

	s.SetBytes(caller)

	for i := range caller {
		caller[i] = 'x'
	}
	clear(caller)

	if got := string(s.Bytes()); got != "caller-owned-secret" {
		t.Errorf("Bytes() = %q after mutating caller's slice; want caller-owned-secret (SetBytes must copy, not alias)", got)
	}
}

func TestSecretBytes_SetBytesZeroizesPriorBuffer(t *testing.T) {
	var s SecretBytes
	s.SetBytes([]byte("first-secret"))

	old := s.Bytes()
	oldCopy := make([]byte, len(old))
	copy(oldCopy, old)

	s.SetBytes([]byte("second-secret"))

	for i, b := range old {
		if b != 0 {
			t.Errorf("old backing[%d] = %d after SetBytes; want 0 (was %q)", i, b, oldCopy[i])
		}
	}

	if got := string(s.Bytes()); got != "second-secret" {
		t.Errorf("Bytes() = %q after second SetBytes; want second-secret", got)
	}
}

func TestSecretBytes_SetBytesWipedByZeroize(t *testing.T) {
	var s SecretBytes
	s.SetBytes([]byte("live-secret"))

	alias := s.Bytes()
	if string(alias) != "live-secret" {
		t.Fatalf("pre-condition: Bytes() = %q; want live-secret", alias)
	}

	s.Zeroize()

	for i, b := range alias {
		if b != 0 {
			t.Errorf("alias[%d] = %d after Zeroize; want 0", i, b)
		}
	}

	if !s.IsEmpty() {
		t.Error("IsEmpty() = false after Zeroize; want true")
	}
}

func TestSecretBytes_BytesAliasWipedByZeroize(t *testing.T) {
	var s SecretBytes
	s.Set("live-secret")

	alias := s.Bytes()
	if string(alias) != "live-secret" {
		t.Fatalf("pre-condition: Bytes() = %q; want live-secret", alias)
	}

	s.Zeroize()

	for i, b := range alias {
		if b != 0 {
			t.Errorf("alias[%d] = %d after Zeroize; want 0", i, b)
		}
	}

	if !s.IsEmpty() {
		t.Error("IsEmpty() = false after Zeroize; want true")
	}
}
