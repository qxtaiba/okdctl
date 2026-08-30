package setup

import "testing"

// Canonical value: https://www.hashicorp.com/trust/security (mirrored at
// apt.releases.hashicorp.com/gpg).
func TestHashiCorpGPGFingerprintConstant(t *testing.T) {
	const canonical = "798AEC654E5C15428C8E42EEAA16FCBCA621E701"
	if expectedHashiCorpGPGFingerprint != canonical {
		t.Errorf("expectedHashiCorpGPGFingerprint = %q, want %q",
			expectedHashiCorpGPGFingerprint, canonical)
	}
}
