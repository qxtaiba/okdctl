package system

import (
	"bytes"
	"testing"
)

func TestZeroBytes(t *testing.T) {
	t.Run("non-empty buffer is zeroed", func(t *testing.T) {
		buf := []byte("super-secret-token")
		ZeroBytes(buf)
		want := make([]byte, len(buf))
		if !bytes.Equal(buf, want) {
			t.Fatalf("ZeroBytes left non-zero bytes: %x", buf)
		}
	})

	t.Run("nil slice is a no-op", func(_ *testing.T) {
		ZeroBytes(nil)
	})

	t.Run("empty slice is a no-op", func(_ *testing.T) {
		ZeroBytes([]byte{})
	})
}
