package setup

import "testing"

// TestHashiCorpGPGFingerprintConstant guards the trust anchor against a typo.
// The canonical value is published at https://www.hashicorp.com/trust/security
// and mirrored at https://apt.releases.hashicorp.com/gpg (HashiCorp Security
// <security@hashicorp.com>). A wrong constant either rejects the genuine key
// or, worse, pins an attacker's key.
func TestHashiCorpGPGFingerprintConstant(t *testing.T) {
	const canonical = "798AEC654E5C15428C8E42EEAA16FCBCA621E701"
	if expectedHashiCorpGPGFingerprint != canonical {
		t.Errorf("expectedHashiCorpGPGFingerprint = %q, want %q",
			expectedHashiCorpGPGFingerprint, canonical)
	}
}
