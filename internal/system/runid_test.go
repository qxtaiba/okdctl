package system

import (
	"regexp"
	"testing"
)

var uuidV4RE = regexp.MustCompile(
	`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`,
)

func TestNewUUIDv4(t *testing.T) {
	t.Run("format matches rfc4122 v4", func(t *testing.T) {
		got := NewUUIDv4()
		if !uuidV4RE.MatchString(got) {
			t.Errorf("NewUUIDv4() = %q; does not match RFC 4122 v4 pattern", got)
		}
	})

	t.Run("consecutive calls are distinct", func(t *testing.T) {
		a := NewUUIDv4()
		b := NewUUIDv4()
		if a == b {
			t.Errorf("two consecutive calls returned identical value %q", a)
		}
	})
}
