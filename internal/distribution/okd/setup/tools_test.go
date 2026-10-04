package setup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/testutil"
)

// Canonical value: https://www.hashicorp.com/trust/security (mirrored at
// apt.releases.hashicorp.com/gpg).
func TestHashiCorpGPGFingerprintConstant(t *testing.T) {
	const canonical = "798AEC654E5C15428C8E42EEAA16FCBCA621E701"
	if expectedHashiCorpGPGFingerprint != canonical {
		t.Errorf("expectedHashiCorpGPGFingerprint = %q, want %q",
			expectedHashiCorpGPGFingerprint, canonical)
	}
}

func TestHashiCorpKeyTrustBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, downloaded, existing string
		wantError                  bool
	}{
		{"new trusted key", expectedHashiCorpGPGFingerprint, "", false},
		{"reuse trusted key", expectedHashiCorpGPGFingerprint, expectedHashiCorpGPGFingerprint, false},
		{"reject downloaded key", "untrusted", "", true},
		{"reject existing key", expectedHashiCorpGPGFingerprint, "untrusted", true},
		{"reject missing fingerprint", "missing", "", true},
		{"reject gpg failure", "failure", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.InstallFakeBin(t, "gpg", `#!/bin/sh
if [ "$1" = "--dearmor" ]; then
 cp "$4" "$3"
 exit $?
fi
for arg in "$@"; do key="$arg"; done
value=$(cat "$key")
case "$value" in
 failure) exit 1 ;;
 missing) echo 'pub:::::::::' ;;
 *) printf 'fpr:::::::::%s:\n' "$value" ;;
esac
`)
			dir := t.TempDir()
			downloaded, destination := filepath.Join(dir, "downloaded"), filepath.Join(dir, "keyring")
			if err := os.WriteFile(downloaded, []byte(tc.downloaded), 0o600); err != nil {
				t.Fatal(err)
			}
			if tc.existing != "" {
				if err := os.WriteFile(destination, []byte(tc.existing), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			err := installHashiCorpKey(t.Context(), downloaded, destination)
			if (err != nil) != tc.wantError {
				t.Fatalf("error=%v", err)
			}
			body, readErr := os.ReadFile(destination)
			if tc.wantError && tc.existing == "" {
				if !errors.Is(readErr, os.ErrNotExist) {
					t.Fatal("refused key installed")
				}
			} else {
				want := tc.existing
				if want == "" {
					want = tc.downloaded
				}
				if readErr != nil || string(body) != want {
					t.Fatal("existing trust changed or new trust missing")
				}
			}
		})
	}
}

func TestHashiCorpKeyVerificationCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := verifyHashiCorpGPGFingerprint(ctx, filepath.Join(t.TempDir(), "key"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("lost cancellation: %v", err)
	}
	if err != nil && strings.Contains(err.Error(), expectedHashiCorpGPGFingerprint) {
		t.Fatal("cancel unexpectedly reached fingerprint parsing")
	}
}
