package config

import (
	"fmt"
	"strings"
	"testing"
)

func TestSecretBytes_String(t *testing.T) {
	t.Run("non-empty buffer returns redacted", func(t *testing.T) {
		var s SecretBytes
		s.Set("hunter2")
		if got := s.String(); got != "[redacted]" {
			t.Errorf("String() = %q; want [redacted]", got)
		}
	})

	t.Run("zero-value returns redacted", func(t *testing.T) {
		var s SecretBytes
		if got := s.String(); got != "[redacted]" {
			t.Errorf("String() = %q; want [redacted]", got)
		}
	})
}

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

func TestSecretBytes_IsEmpty(t *testing.T) {
	var s SecretBytes

	if !s.IsEmpty() {
		t.Error("IsEmpty() = false on zero value; want true")
	}

	s.Set("something")
	if s.IsEmpty() {
		t.Error("IsEmpty() = true after Set; want false")
	}

	s.Zeroize()
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
