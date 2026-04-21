package flux

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

// verifyKnownHostsFingerprint compares the SHA256 of scanOutput against
// fs.KnownHostsSHA256. When the setting is empty and fs.AcceptHostKey is
// false, returns a ConfigError refusing to pin an unverified host key.
// When AcceptHostKey is true but no fingerprint is set, logs a WARN with
// the observed fingerprint so the operator can pin it on the next run.
func verifyKnownHostsFingerprint(scanOutput string, fs Settings, logger *slog.Logger, host string) error {
	sum := sha256.Sum256([]byte(scanOutput))
	observed := hex.EncodeToString(sum[:])

	expected := normalizeHostKeyFingerprint(fs.KnownHostsSHA256)
	if expected == "" {
		if !fs.AcceptHostKey {
			return &errtypes.ConfigError{
				Msg: fmt.Sprintf(
					"flux: refusing to pin unverified host key for %s; "+
						"set addons.flux.settings.known_hosts_sha256=%s "+
						"or addons.flux.settings.accept_host_key=true to proceed",
					host, observed),
			}
		}
		logger.Warn("flux: host-key fingerprint not pinned — accepting observed key",
			"host", host, "observed_sha256", observed,
			"hint", "set addons.flux.settings.known_hosts_sha256 on the next run to pin")
		return nil
	}

	if expected != observed {
		return &errtypes.ConfigError{
			Msg: fmt.Sprintf("flux: host-key fingerprint mismatch for %s (expected %s, observed %s)",
				host, expected, observed),
		}
	}
	logger.Info("flux: host-key fingerprint verified", "host", host)
	return nil
}

func normalizeHostKeyFingerprint(s string) string {
	s = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "sha256:")
	return strings.ReplaceAll(s, ":", "")
}
