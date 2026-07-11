package system

import (
	"crypto/rand"
	"encoding/hex"
)

// NewUUIDv4 returns a random RFC 4122 version-4 UUID string, used for
// run/bundle IDs (cli/root.go, cli/debug_bundle.go). Lives here rather than
// a new one-function package because it has no narrower existing home and
// is dependency-light like the rest of this package.
// Panics if crypto/rand cannot supply entropy — callers treat this
// as a fatal runtime condition, matching google/uuid.NewString's
// contract for security-critical random sources.
func NewUUIDv4() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("system.NewUUIDv4: crypto/rand unavailable: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	var out [36]byte
	hex.Encode(out[0:8], b[0:4])
	out[8] = '-'
	hex.Encode(out[9:13], b[4:6])
	out[13] = '-'
	hex.Encode(out[14:18], b[6:8])
	out[18] = '-'
	hex.Encode(out[19:23], b[8:10])
	out[23] = '-'
	hex.Encode(out[24:36], b[10:16])
	return string(out[:])
}
